package share

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"friendnet.org/client/storage"
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

// ErrServerManagerClosed is returned by ServerShareManager methods when it is closed.
var ErrServerManagerClosed = errors.New("server manager is closed")

// ErrShareExists is returned when trying to create a new share with a name that already exists.
var ErrShareExists = errors.New("share with same name exists")

// ErrIndexingDisabled is returned when trying to index a share that has indexing disabled.
var ErrIndexingDisabled = errors.New("indexing disabled for share")

type shareAndRecord struct {
	share  Share
	record storage.ShareRecord
}

// ServerShareManager manages shares for a server.
type ServerShareManager struct {
	mu       sync.RWMutex
	isClosed bool

	logger *slog.Logger

	ctx       context.Context
	ctxCancel context.CancelFunc

	serverUuid string
	storage    *storage.Storage

	// A mapping of share names to their underlying Share instances.
	shareMap map[string]shareAndRecord

	indexerInterval time.Duration
	indexingShares  map[string]struct{}
}

// NewServerShareManager creates a new share manager for the given server.
// It gets share records for the server and instantiates Share instances for them.
func NewServerShareManager(
	logger *slog.Logger,
	serverUuid string,
	storage *storage.Storage,
) (*ServerShareManager, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	// Get shares for server.
	records, err := storage.GetSharesByServer(ctx, serverUuid)
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf(`failed to get share records for server %q: %w`, serverUuid, err)
	}

	shareMap := make(map[string]shareAndRecord, len(records))
	for _, record := range records {
		var share Share
		share, err = NewDirShare(record.Name, record.Path.String())
		shareMap[record.Name] = shareAndRecord{
			share:  share,
			record: record,
		}
	}

	m := &ServerShareManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		logger: logger,

		serverUuid: serverUuid,
		storage:    storage,

		shareMap: shareMap,

		indexerInterval: 10 * time.Minute,
		indexingShares:  make(map[string]struct{}),
	}

	go m.indexerDaemon()

	return m, nil
}

func (m *ServerShareManager) snapshotSharesNoLock() []Share {
	slice := make([]Share, 0, len(m.shareMap))
	for _, share := range m.shareMap {
		slice = append(slice, share.share)
	}
	return slice
}

func (m *ServerShareManager) indexerDaemon() {
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

		for _, rec := range recs {
			m.doIndexShare(rec)
		}
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

func (m *ServerShareManager) doIndexShare(rec storage.ShareRecord) {
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
		"uuid", rec.Uuid,
		"name", rec.Name,
	)

	count, _, idxErr := m.IndexShare(m.ctx, rec.Name)
	if idxErr != nil {
		m.logger.Error("failed to index share",
			"uuid", rec.Uuid,
			"name", rec.Name,
			"err", idxErr,
		)
		return
	}

	m.logger.Info("indexed share",
		"uuid", rec.Uuid,
		"name", rec.Name,
		"file_count", count,
	)
}

// GetAll returns all current shares for the server.
// Returns empty if the manager is closed.
// Note that this method creates a new slice each time it is called.
func (m *ServerShareManager) GetAll() []Share {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil
	}

	return m.snapshotSharesNoLock()
}

// GetByName returns the share with the specified name and true, or nil and false if no such share name exists.
// Always returns nil and false if the manager is closed.
func (m *ServerShareManager) GetByName(name string) (Share, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil, false
	}

	share, has := m.shareMap[name]
	return share.share, has
}

// Add creates a new server share.
// If a share with the same name exists, returns ErrShareExists.
// Triggers an index in the background when the share is created.
func (m *ServerShareManager) Add(ctx context.Context, name string, path string) (Share, error) {
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
	err := m.storage.CreateShare(ctx, m.serverUuid, name, path)
	if err != nil {
		return nil, fmt.Errorf(`failed to create new share %q: %w`, name, err)
	}

	// Get record.
	rec, _, err := m.storage.GetShareByServerUuidAndName(ctx, m.serverUuid, name)
	if err != nil {
		return nil, fmt.Errorf(`failed to get share record for newly created share %q: %w`, name, err)
	}

	// Create instance.
	share, err := NewDirShare(name, path)
	if err != nil {
		return nil, fmt.Errorf(`failed to create share instance for newly created share %q: %w`, name, err)
	}

	m.mu.Lock()
	m.shareMap[name] = shareAndRecord{
		share:  share,
		record: rec,
	}
	m.mu.Unlock()

	if rec.EnableIndexing {
		go func() {
			m.doIndexShare(rec)
		}()
	}

	return share, nil
}

// Delete deletes an existing server share.
// If the share does not exist, this is no-op.
func (m *ServerShareManager) Delete(ctx context.Context, name string) error {
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

// IndexShare indexes all files in the share with the specified name.
// It returns the number of files indexed, whether the share existed, and any error that occurred.
// Refuses to index the share if it has indexing disabled, returning ErrIndexingDisabled.
func (m *ServerShareManager) IndexShare(ctx context.Context, name string) (count int, hasShare bool, err error) {
	m.mu.RLock()
	if m.isClosed {
		m.mu.RUnlock()
		return 0, false, ErrServerManagerClosed
	}
	val, has := m.shareMap[name]
	m.mu.RUnlock()

	if !has {
		return 0, false, nil
	}

	share := val.share
	rec := val.record

	if !rec.EnableIndexing {
		return 0, true, ErrIndexingDisabled
	}

	err = m.storage.ClearShareIndex(ctx, rec.Uuid)
	if err != nil {
		return count, true, fmt.Errorf(`failed to clear share %q index before indexing: %w`, rec.Uuid, err)
	}

	dirs := []string{"/"}

	for len(dirs) > 0 {
		dir := dirs[0]
		dirs = dirs[1:]

		var files []*pb.MsgFileMeta
		files, err = share.DirFiles(common.UncheckedCreateProtoPath(dir))
		for _, file := range files {
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
				path,
				file.IsDir,
				int64(file.Size),
			)
			if err != nil {
				return count, true, fmt.Errorf(`failed to insert share %q index for file %q: %w`, rec.Uuid, path, err)
			}
		}
	}

	return count, true, nil
}

// Close closes all shares managed by the manager, then the manager itself.
func (m *ServerShareManager) Close() error {
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
	var wg sync.WaitGroup
	for _, share := range shares {
		wg.Go(func() {
			_ = share.Close()
		})
	}
	wg.Wait()

	return nil
}

// SearchShares searches the indexes of shares managed by the manager for the specified query.
// It returns a slice of search results.
// Shares that have indexing disabled will not be searched.
func (m *ServerShareManager) SearchShares(ctx context.Context, query string, limit int64) ([]pb.MsgSearchResult, error) {
	m.mu.RLock()
	if m.isClosed {
		m.mu.RUnlock()
		return nil, ErrServerManagerClosed
	}

	uuids := make([]string, 0, len(m.shareMap))
	uuidToShare := make(map[string]Share)
	for _, share := range m.shareMap {
		if !share.record.EnableIndexing {
			continue
		}

		uuids = append(uuids, share.record.Uuid)
		uuidToShare[share.record.Uuid] = share.share
	}
	m.mu.RUnlock()

	recs, err := m.storage.QueryShareIndexByShareUuids(ctx, uuids, query, limit)
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

		result := &results[i]
		result.DirectoryPath = "/" + share.Name() + rec.Path.String()
		result.File = meta
	}

	return results, nil
}
