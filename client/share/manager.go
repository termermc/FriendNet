package share

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"syscall"
	"time"

	"friendnet.org/client/storage"
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

// ErrServerManagerClosed is returned by Manager methods when it is closed.
var ErrServerManagerClosed = errors.New("server manager is closed")

// ErrShareExists is returned when trying to create a new share with a name that already exists.
var ErrShareExists = errors.New("share with same name exists")

// ErrIndexingDisabled is returned when trying to index a share that has indexing disabled.
var ErrIndexingDisabled = errors.New("indexing disabled for share")

// ErrTooManyFiles is returned when trying to index a share that has too many files.
var ErrTooManyFiles = errors.New("too many files in share, indexing canceled")

type shareData struct {
	share       Share
	record      storage.ShareRecord
	lastIndexId int64
}

// Manager manages shares for a server.
type Manager struct {
	mu       sync.RWMutex
	isClosed bool

	logger *slog.Logger

	ctx       context.Context
	ctxCancel context.CancelFunc

	serverUuid string
	storage    *storage.Storage

	// A mapping of share names to their underlying Share instances.
	shareMap map[string]*shareData

	indexerInterval         time.Duration
	indexingShares          map[string]struct{}
	indexerMaxFiles         int
	orphanedIndexGcInterval time.Duration
}

// NewManager creates a new share manager for the given server.
// It gets share records for the server and instantiates Share instances for them.
func NewManager(
	logger *slog.Logger,
	serverUuid string,
	storage *storage.Storage,
) (*Manager, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	// Get shares for server.
	records, err := storage.GetSharesByServer(ctx, serverUuid)
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf(`failed to get share records for server %q: %w`, serverUuid, err)
	}

	shareMap := make(map[string]*shareData, len(records))
	for _, record := range records {
		var share Share
		share, err = NewDirShare(
			record.Name,
			record.Path.String(),
			record.FollowLinks,
		)
		shareMap[record.Name] = &shareData{
			share:  share,
			record: record,
		}
	}

	m := &Manager{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		logger: logger,

		serverUuid: serverUuid,
		storage:    storage,

		shareMap: shareMap,

		indexerInterval:         1 * time.Hour,
		indexingShares:          make(map[string]struct{}),
		indexerMaxFiles:         1_000_000,
		orphanedIndexGcInterval: 10 * time.Minute,
	}

	go m.indexerDaemon()
	go m.orphanedIndexGc()

	return m, nil
}

func (m *Manager) snapshotSharesNoLock() []Share {
	slice := make([]Share, 0, len(m.shareMap))
	for _, share := range m.shareMap {
		slice = append(slice, share.share)
	}
	return slice
}

func (m *Manager) indexerDaemon() {
	defer func() {
		if rec := recover(); rec != nil {
			m.logger.Error("share indexer daemon panicked",
				"err", rec,
			)
		}
	}()

	do := func() {
		m.mu.RLock()
		recs := make([]storage.ShareRecord, 0, len(m.shareMap))
		for _, val := range m.shareMap {
			recs = append(recs, val.record)
		}
		m.mu.RUnlock()

		var wg sync.WaitGroup
		for _, rec := range recs {
			wg.Go(func() {
				m.indexShareWithLockAndLogging(rec)
			})
		}
		wg.Wait()
	}

	do()

	ticker := time.NewTicker(m.indexerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			do()
		}
	}
}

func (m *Manager) orphanedIndexGc() {
	do := func() {
		var totalDeleted int64
		for {
			deleted, err := m.storage.ClearOrphanedShareIndexes(m.ctx, 100)
			if err != nil {
				m.logger.Error("failed to clear orphaned indexes",
					"service", "share.Manager",
					"err", err,
				)
				break
			}

			if deleted == 0 {
				break
			}

			totalDeleted += deleted
		}

		if totalDeleted > 0 {
			m.logger.Info("deleted orphaned share indexes",
				"service", "share.Manager",
				"total", totalDeleted,
			)
		}
	}

	do()

	ticker := time.NewTicker(m.orphanedIndexGcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			do()
		}
	}
}

// indexShare indexes all files in the share with the specified name.
// It returns the number of files indexed, whether the share existed, and any error that occurred.
// Refuses to index the share if it has indexing disabled, returning ErrIndexingDisabled.
func (m *Manager) indexShare(ctx context.Context, name string) (count int, hasShare bool, err error) {
	m.mu.Lock()
	val, has := m.shareMap[name]
	if !has {
		m.mu.Unlock()
		return 0, false, nil
	}
	curIndexId := time.Now().UnixMilli()
	val.lastIndexId = curIndexId
	m.mu.Unlock()

	share := val.share
	rec := val.record

	if !rec.EnableIndexing {
		return 0, true, ErrIndexingDisabled
	}

	shouldClearOld := false
	shouldOptimize := false
	defer func() {
		if !shouldClearOld {
			return
		}

		err = m.storage.ClearShareIndex(ctx, rec.Uuid, curIndexId)
		if err != nil {
			m.logger.Error("failed to clear old indexes for share",
				"service", "share.Manager",
				"share_uuid", rec.Uuid,
				"err", err,
			)
			return
		}

		if shouldOptimize {
			optErr := m.storage.OptimizeShareIndex(ctx)
			if optErr != nil {
				m.logger.Warn("failed to optimize share index",
					"service", "share.Manager",
					"share_uuid", rec.Uuid,
					"err", optErr,
				)
			}
		}
	}()

	dirs := []string{"/"}

	for len(dirs) > 0 {
		dir := dirs[0]
		dirs = dirs[1:]

		var files []*pb.MsgFileMeta
		files, err = share.DirFiles(common.UncheckedCreateProtoPath(dir))
		if err != nil {
			// Skip files that were removed or we do not have permission to access.
			if os.IsNotExist(err) || os.IsPermission(err) || errors.Is(err, syscall.ESRCH) {
				continue
			}

			return count, true, fmt.Errorf("failed to read directory %q: %w", dir, err)
		}
		for _, file := range files {
			if count >= m.indexerMaxFiles {
				shouldClearOld = true
				return count, true, ErrTooManyFiles
			}

			count++

			var path string
			if dir == "/" {
				path = "/" + file.Name
			} else {
				path = dir + "/" + file.Name
			}

			if file.IsDir {
				dirs = append(dirs, path)
			}

			err = m.storage.InsertShareIndex(ctx,
				rec.Uuid,
				curIndexId,
				path,
				file.IsDir,
				int64(file.Size),
			)
			if err != nil {
				return count, true, fmt.Errorf(`failed to insert share %q index for file %q: %w`, rec.Uuid, path, err)
			}
		}
	}

	shouldClearOld = true
	shouldOptimize = true

	return count, true, nil
}

func (m *Manager) indexShareWithLockAndLogging(rec storage.ShareRecord) {
	m.mu.Lock()
	_, has := m.indexingShares[rec.Uuid]
	if has {
		m.mu.Unlock()
		return
	}
	m.indexingShares[rec.Uuid] = struct{}{}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.indexingShares, rec.Uuid)
		m.mu.Unlock()
	}()

	m.logger.Info("indexing share",
		"service", "share.Manager",
		"uuid", rec.Uuid,
		"name", rec.Name,
		"path", rec.Path,
	)

	count, _, idxErr := m.indexShare(m.ctx, rec.Name)
	if idxErr != nil {
		if errors.Is(idxErr, ErrIndexingDisabled) {
			m.logger.Info("indexing disabled for share",
				"service", "share.Manager",
				"uuid", rec.Uuid,
				"name", rec.Name,
				"path", rec.Path,
			)
			return
		}
		if errors.Is(idxErr, ErrTooManyFiles) {
			m.logger.Warn("share has too many files, indexing canceled",
				"service", "share.Manager",
				"uuid", rec.Uuid,
				"name", rec.Name,
				"path", rec.Path,
				"file_count", count,
			)
			return
		}

		m.logger.Error("failed to index share",
			"service", "share.Manager",
			"uuid", rec.Uuid,
			"name", rec.Name,
			"path", rec.Path,
			"err", idxErr,
		)
		return
	}

	m.logger.Info("indexed share",
		"uuid", rec.Uuid,
		"name", rec.Name,
		"path", rec.Path,
		"file_count", count,
	)
}

// ScheduleShareIndex schedules an index of the share with the specified name.
// If the share does not exist, this is no-op.
// If the share has indexing disabled, returns ErrIndexingDisabled.
// If the manager is closed, returns ErrServerManagerClosed.
func (m *Manager) ScheduleShareIndex(name string) error {
	m.mu.RLock()
	if m.isClosed {
		m.mu.RUnlock()
		return ErrServerManagerClosed
	}
	val, has := m.shareMap[name]
	m.mu.RUnlock()
	if !has {
		return nil
	}

	if !val.record.EnableIndexing {
		return ErrIndexingDisabled
	}

	go func() {
		m.indexShareWithLockAndLogging(val.record)
	}()
	return nil
}

// GetAll returns all current shares for the server.
// Returns empty if the manager is closed.
// Note that this method creates a new slice each time it is called.
func (m *Manager) GetAll() []Share {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil
	}

	return m.snapshotSharesNoLock()
}

// GetByName returns the share with the specified name and true, or nil and false if no such share name exists.
// Always returns nil and false if the manager is closed.
func (m *Manager) GetByName(name string) (Share, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil, false
	}

	share, has := m.shareMap[name]
	if !has {
		return nil, false
	}
	return share.share, true
}

// Add creates a new server share.
// If a share with the same name exists, returns ErrShareExists.
// Triggers an index in the background when the share is created.
func (m *Manager) Add(
	ctx context.Context,
	name string,
	path string,
	followLinks bool,
) (Share, error) {
	m.mu.Lock()

	if m.isClosed {
		m.mu.Unlock()
		return nil, ErrServerManagerClosed
	}

	_, exists := m.shareMap[name]
	m.mu.Unlock()

	if exists {
		return nil, ErrShareExists
	}

	// Create in storage.
	err := m.storage.CreateShare(ctx, m.serverUuid, name, path, followLinks)
	if err != nil {
		return nil, fmt.Errorf(`failed to create new share %q: %w`, name, err)
	}

	// Get record.
	rec, _, err := m.storage.GetShareByServerUuidAndName(ctx, m.serverUuid, name)
	if err != nil {
		return nil, fmt.Errorf(`failed to get share record for newly created share %q: %w`, name, err)
	}

	// Create instance.
	share, err := NewDirShare(
		name,
		path,
		followLinks,
	)
	if err != nil {
		return nil, fmt.Errorf(`failed to create share instance for newly created share %q: %w`, name, err)
	}

	m.mu.Lock()
	m.shareMap[name] = &shareData{
		share:  share,
		record: rec,
	}
	m.mu.Unlock()

	if rec.EnableIndexing {
		go func() {
			m.indexShareWithLockAndLogging(rec)
		}()
	}

	return share, nil
}

// Delete deletes an existing server share.
// If the share does not exist, this is no-op.
func (m *Manager) Delete(ctx context.Context, name string) error {
	m.mu.Lock()
	if m.isClosed {
		m.mu.Unlock()
		return ErrShareClosed
	}

	share, has := m.shareMap[name]

	m.mu.Unlock()

	if !has {
		return nil
	}

	// Remove from storage.
	err := m.storage.DeleteShareByServerUuidAndName(ctx, m.serverUuid, name)
	if err != nil {
		return fmt.Errorf(`failed to remove share with server UUID %q and name %q: %w`, m.serverUuid, name, err)
	}

	// Close share and remove it from map.
	_ = share.share.Close()
	m.mu.Lock()
	delete(m.shareMap, name)
	m.mu.Unlock()

	return nil
}

// Close closes all shares managed by the manager, then the manager itself.
func (m *Manager) Close() error {
	m.mu.Lock()

	if m.isClosed {
		m.mu.Unlock()
		return nil
	}
	m.isClosed = true

	shares := m.snapshotSharesNoLock()

	m.mu.Unlock()

	m.ctxCancel()

	// Close all shares.
	for _, share := range shares {
		_ = share.Close()
	}

	return nil
}

// SearchShares searches the indexes of shares managed by the manager for the specified query.
// It returns a slice of search results.
// Shares that have indexing disabled will not be searched.
func (m *Manager) SearchShares(ctx context.Context, query string, limit int64) ([]pb.MsgSearchResult, error) {
	m.mu.RLock()
	if m.isClosed {
		m.mu.RUnlock()
		return nil, ErrServerManagerClosed
	}

	indexIds := make([]int64, 0, len(m.shareMap))
	uuids := make([]string, 0, len(m.shareMap))
	uuidToShare := make(map[string]Share)
	for _, share := range m.shareMap {
		if !share.record.EnableIndexing {
			continue
		}

		indexIds = append(indexIds, share.lastIndexId)
		uuids = append(uuids, share.record.Uuid)
		uuidToShare[share.record.Uuid] = share.share
	}
	m.mu.RUnlock()

	recs, err := m.storage.QueryShareIndexByShareUuids(ctx, uuids, indexIds, query, limit)
	if err != nil {
		return nil, fmt.Errorf(`failed to search shares: %w`, err)
	}

	metas := make([]pb.MsgFileMeta, len(recs))
	results := make([]pb.MsgSearchResult, len(recs))
	for i, rec := range recs {
		share := uuidToShare[rec.Share]

		meta := &metas[i]
		meta.Name = rec.Path.Name()
		meta.IsDir = rec.IsDirectory
		meta.Size = uint64(rec.Size)

		segments := rec.Path.ToSegments()
		var dirPath common.ProtoPath
		dirPath, err = common.SegmentsToPath(segments[:len(segments)-1])
		if err != nil {
			return nil, fmt.Errorf(`failed to convert segments to path: %w`, err)
		}

		result := &results[i]
		if dirPath.IsRoot() {
			result.DirectoryPath = "/" + share.Name()
		} else {
			result.DirectoryPath = "/" + share.Name() + dirPath.String()
		}
		result.File = meta
		result.Snippet = rec.Snippet
	}

	return results, nil
}
