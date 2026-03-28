// The download manager is responsible for queuing downloads, downloading them, and propagating status updates to the
// client RPC and the peers that are being downloaded from.
// It is intended to be a global component that can manage downloads over multiple servers using a MultiClient instance.
//
// All downloads, regardless of their status, are stored in a global slice. Downloaders periodically scan the slice for
// downloads in the QUEUED status and start working on them, changing their status to PENDING. The slice is not a queue;
// it is a global list with long-lived state structs that have statuses.
//
// The decision not to use a queue came from the need to snapshot all downloads, regardless of status, and send them to
// the client RPC is requested. If a queue was used, we would have to query all workers for downloads they own as well
// as snapshotting the global queue. That makes a real queue infeasible. The global slice scanning design is slow, but
// it reduces complexity and should be suitable for <1,000 pending downloads, which I expect to be the case in the real
// world.
//
// When a downloader takes ownership of a download, it reports its progress by putting it into a global status update
// channel. The channel is consumed by a goroutine that processes the update and sends out the necessary messages. The
// channel is buffered, so if the channel is full, updates are discarded until it is drained enough to accept new
// updates.
//
// The update processor goroutine, in addition to sending out status update messages to peers and the client RPC, also
// updates the client database. This allows download state to be restored when the client is restarted.

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"friendnet.org/client/event"
	"friendnet.org/client/fsys"
	"friendnet.org/client/room"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/protocol"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/google/uuid"
)

// DmDirIncompleteSetting is the setting key for the download manager's incomplete download directory.
// Client must be restarted for it to take effect.
const DmDirIncompleteSetting = "dm_dir_incomplete"

// DmDirCompleteSetting is the setting key for the download manager's complete download directory.
// Client must be restarted for it to take effect.
const DmDirCompleteSetting = "dm_dir_complete"

// DmDlConcurrencySetting is the setting key for the number of concurrent downloads to launch.
// Updates to this will reflect immediately.
const DmDlConcurrencySetting = "dm_dl_concurrency"

type dmUpdate struct {
	rpc *v1.DownloadStatusUpdate
	ds  *DownloadHandle
}

func (u *dmUpdate) ToProto() *pb.MsgDownloadStatusUpdate {
	return &pb.MsgDownloadStatusUpdate{
		Path: u.ds.filePath.String(),

		// Enum is duplicate and can be casted directly.
		Status: pb.DownloadStatus(u.rpc.Status),

		BytesDownloaded: u.rpc.Downloaded,
	}
}

var errHandleStopped = errors.New("handle stopped")
var errIsDir = errors.New("is a directory")

// DownloadHandle is a handle for a download.
type DownloadHandle struct {
	dm *DownloadManager

	status atomic.Pointer[pb.DownloadStatus]

	// stopFnOrNil is a function that can be called to stop a pending download and set it to a
	// specific status.
	// It may be nil.
	stopFnOrNil atomic.Pointer[func(pb.DownloadStatus)]

	// The file download's UUID.
	uuid string

	// The server the file is being downloaded from.
	server *Server

	// The peer on the server the file is being downloaded from.
	peer common.NormalizedUsername

	// The file's path within the peer.
	filePath common.ProtoPath

	// The file's total size, in bytes.
	// If the file's size changes from this when resuming, the file changed.
	// If the size is -1, it needs to be fetched.
	fileTotalSize atomic.Int64

	// The file's current download progress.
	fileDownloadedBytes atomic.Uint64

	// The download error message, if any.
	errorMessage atomic.Pointer[string]
}

// DownloadManager manages downloads across multiple servers.
// It can resume and retry downloads, even when the client is closed and reopened, or when a peer goes offline and
// comes back later.
// It is designed to work similarly to the download manager in Nicotine+.
//
// In the completed folder, the directory structure is as follows:
// `/<peer username>-<server UUID>/<peer path>...`
//
// So if you download "/music/song.mp3" from "jimmy" on server "abcd1234", the file will be saved at path:
// `/jimmy-abcd1234/music/song.mp3`
type DownloadManager struct {
	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	incompleteFnReplacer fsys.FilenameReplacer
	completeFnReplacer   fsys.FilenameReplacer

	logger *slog.Logger

	multi    *MultiClient
	eventBus *event.Bus
	storage  *storage.Storage

	dirIncomplete string
	dirComplete   string

	handles []*DownloadHandle

	// A queue of pending download progress events to send to the event bus.
	// It is buffered, but sends should be discarded if the buffer is full instead of blocking.
	pendingUpdates chan dmUpdate

	// The current number of active workers.
	activeWorkers atomic.Int64
}

func NewDownloadManager(
	logger *slog.Logger,

	multi *MultiClient,
	eventBus *event.Bus,
	storage *storage.Storage,
) (*DownloadManager, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	homeDir, err := os.UserHomeDir()
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf(`failed to determine user home directory: %w`, err)
	}

	defDlBaseDir := filepath.Join(homeDir, "Downloads", "FriendNet Downloads")
	defDlIncomplete := filepath.Join(defDlBaseDir, "Incomplete")
	defDlComplete := filepath.Join(defDlBaseDir, "Complete")

	// Get settings.
	dirIncomplete, err := storage.GetSettingOrPut(ctx, DmDirIncompleteSetting, defDlIncomplete)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	dirComplete, err := storage.GetSettingOrPut(ctx, DmDirCompleteSetting, defDlComplete)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// Get filename replacers for paths.
	incompleteFnReplacer, err := fsys.GetFilenameReplacerForPath(dirIncomplete)
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf(`failed to get filename replacer for incomplete downloads directory %q: %w`, dirIncomplete, err)
	}
	completeFnReplacer, err := fsys.GetFilenameReplacerForPath(dirComplete)
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf(`failed to get filename replacer for complete downloads directory %q: %w`, dirComplete, err)
	}

	dm := &DownloadManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		incompleteFnReplacer: incompleteFnReplacer,
		completeFnReplacer:   completeFnReplacer,

		logger: logger,

		multi:    multi,
		eventBus: eventBus,
		storage:  storage,

		dirIncomplete: dirIncomplete,
		dirComplete:   dirComplete,

		handles: nil,

		pendingUpdates: make(chan dmUpdate, 100),
	}

	// Load handles.
	records, err := storage.GetDownloadStates(ctx)
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf("failed to load download handles: %w", err)
	}
	states := make([]*DownloadHandle, 0, len(records))
	for _, rec := range records {
		srv, has := multi.GetByUuid(rec.Server)
		if !has {
			logger.Warn("download state record refers to nonexistent server",
				"service", "client.DownloadManager",
				"server_uuid", rec.Server,
			)
			continue
		}

		state := DownloadHandle{
			dm:       dm,
			uuid:     rec.Uuid,
			server:   srv,
			peer:     rec.PeerUsername,
			filePath: rec.FilePath,
		}
		state.status.Store(&rec.Status)
		state.fileTotalSize.Store(rec.FileTotalSize)
		state.fileDownloadedBytes.Store(uint64(rec.FileDownloadedBytes))
		state.errorMessage.Store(rec.Error)

		states = append(states, &state)
	}

	dm.handles = states

	go dm.downloader()
	go dm.updateDrainer()

	return dm, nil
}

func (dm *DownloadManager) downloader() {
	ticker := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			// Fetch the current download concurrency setting.
			// We fetch this on-demand because this should be able to be changed at runtime.
			dlConcurrency, settingErr := dm.storage.GetSettingIntOrPut(dm.ctx, DmDlConcurrencySetting, 4)
			if settingErr != nil {
				dm.logger.Error("failed to get download concurrency setting",
					"service", "client.DownloadManager",
					"err", settingErr,
				)
			}
			if dlConcurrency < 1 {
				dlConcurrency = 1
			}

			dm.mu.RLock()

			launched := dm.activeWorkers.Load()
			for _, state := range dm.handles {
				if launched >= dlConcurrency {
					break
				}

				if *state.status.Load() == pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED {
					go func() {
						dlErr := dm.startDownload(state)
						if dlErr != nil {
							dm.logger.Error("failed to download queued file",
								"service", "client.DownloadManager",
								"server_uuid", state.server.Uuid,
								"peer_username", state.peer.String(),
								"file_path", state.filePath.String(),
								"err", dlErr,
							)
						}
					}()
					launched++
				}
			}

			dm.mu.RUnlock()
		}
	}
}

func (dm *DownloadManager) updateDrainer() {
	var mu sync.Mutex
	buf := make([]dmUpdate, 0)

	go func() {
		// Goroutine that batches updates.

		ticker := time.NewTicker(1 * time.Second)

		for {
			select {
			case <-dm.ctx.Done():
				return
			case <-ticker.C:
				var updates []dmUpdate
				mu.Lock()
				if len(buf) == 0 {
					mu.Unlock()
					continue
				}
				updates = make([]dmUpdate, len(buf))
				copy(updates, buf)
				buf = buf[:0]
				mu.Unlock()

				// Sort updates by server UUID.
				byServer := make(map[string][]dmUpdate)
				for _, upd := range updates {
					byServer[upd.ds.server.Uuid] = append(byServer[upd.ds.server.Uuid], upd)
				}

				// Send batched client RPC messages.
				for server, upds := range byServer {
					pub := dm.eventBus.CreatePublisher(&v1.EventContext{
						ServerUuid: server,
					})

					files := make([]*v1.DownloadStatusUpdate, len(upds))
					for i, upd := range upds {
						files[i] = upd.rpc
					}

					pub.Publish(&v1.Event{
						Type: v1.Event_TYPE_DOWNLOAD_STATUS_UPDATES,
						DownloadStatusUpdates: &v1.Event_DownloadStatusUpdates{
							Files: files,
						},
					})
				}

				// Send batched peer notifications.
				for _, serverUpds := range byServer {
					server := serverUpds[0].ds.server

					_ = server.TryDo(func(conn *room.Conn) error {
						// Sort by peer.
						byPeer := make(map[common.NormalizedUsername][]dmUpdate)
						for _, upd := range serverUpds {
							byPeer[upd.ds.peer] = append(byPeer[upd.ds.peer], upd)
						}

						// Send updates to peers.
						for username, upds := range byPeer {
							peer := conn.GetVirtualC2cConn(username, false)

							go func() {
								bidi, err := peer.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_DOWNLOAD_STATUS_UPDATE, upds[0].ToProto())
								if err != nil {
									return
								}
								defer func() {
									_ = bidi.Close()
								}()

								for _, upd := range upds[1:] {
									_ = bidi.Write(pb.MsgType_MSG_TYPE_DOWNLOAD_STATUS_UPDATE, upd.ToProto())
								}
							}()
						}

						return nil
					})
				}

				// Write to DB.
				for _, upd := range updates {
					err := dm.storage.UpdateDownloadState(
						upd.ds.uuid,
						*upd.ds.status.Load(),
						upd.ds.fileTotalSize.Load(),
						int64(upd.ds.fileDownloadedBytes.Load()),
						upd.ds.errorMessage.Load(),
					)
					if err != nil {
						dm.logger.Error("failed to update download state in database",
							"service", "client.DownloadManager",
							"uuid", upd.ds.uuid,
							"status", upd.ds.status.Load().String(),
							"fileTotalSize", upd.ds.fileTotalSize.Load(),
							"fileDownloadedBytes", upd.ds.fileDownloadedBytes.Load(),
							"errorStr", upd.ds.errorMessage.Load(),
						)
					}
				}
			}
		}
	}()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case upd := <-dm.pendingUpdates:
			mu.Lock()
			buf = append(buf, upd)
			mu.Unlock()
		}
	}
}

func (dm *DownloadManager) Close() error {
	dm.mu.Lock()
	if dm.isClosed {
		dm.mu.Unlock()
		return nil
	}
	dm.isClosed = true
	dm.mu.Unlock()

	dm.ctxCancel()
	return nil
}

func (dm *DownloadManager) SnapshotStates() []*v1.DownloadManagerItem {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	items := make([]*v1.DownloadManagerItem, len(dm.handles))
	for i, state := range dm.handles {
		items[i] = &v1.DownloadManagerItem{
			Type:         v1.DownloadManagerItem_TYPE_DOWNLOAD,
			Uuid:         state.uuid,
			ServerUuid:   state.server.Uuid,
			PeerUsername: state.peer.String(),
			FilePath:     state.filePath.String(),
			Download: &v1.DownloadManagerItem_Download{
				Status:       v1.DownloadStatus(*state.status.Load()),
				Downloaded:   state.fileDownloadedBytes.Load(),
				FileSize:     state.fileTotalSize.Load(),
				ErrorMessage: state.errorMessage.Load(),
			},
		}
	}

	return items
}

func (dm *DownloadManager) getByUuid(uuid string) (*DownloadHandle, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for _, state := range dm.handles {
		if state.uuid == uuid {
			return state, true
		}
	}

	return nil, false
}

// Queue queues a new file download.
// If there is a pending or queued entry for the same file already, this function is no-op.
func (dm *DownloadManager) Queue(
	server *Server,
	peer common.NormalizedUsername,
	filePath common.ProtoPath,
) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var uid string
	replaceSlot := -1

	// Search for a duplicate entry.
	for i, state := range dm.handles {
		if state.server == server && state.peer == peer && state.filePath == filePath {
			// Is it canceled or failed?
			switch *state.status.Load() {
			case pb.DownloadStatus_DOWNLOAD_STATUS_CANCELED:
				fallthrough
			case pb.DownloadStatus_DOWNLOAD_STATUS_ERROR:
				// Replace state.
				replaceSlot = i
				uid = state.uuid
				break
			default:
				// Already exists and not a candidate for replacement.
				return nil
			}
		}
	}

	if uid == "" {
		uidRaw, err := uuid.NewV7()
		if err != nil {
			panic(err)
		}
		uid = uidRaw.String()
	}

	// Create new state.
	state := &DownloadHandle{
		dm:       dm,
		uuid:     uid,
		server:   server,
		peer:     peer,
		filePath: filePath,
	}

	state.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
	state.fileTotalSize.Store(-1)
	state.errorMessage.Store(nil)

	err := dm.storage.CreateDownloadState(
		dm.ctx,
		uid,
		server.Uuid,
		peer,
		pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED,
		filePath,
	)
	if err != nil {
		return fmt.Errorf(`failed to create download state for UUID %s: %w`, uid, err)
	}

	if replaceSlot == -1 {
		dm.handles = append(dm.handles, state)
	} else {
		dm.handles[replaceSlot] = state
	}

	return nil
}

// Remove removes the item with the specified UUID.
// If the item did not exist, returns false.
// Only returns an error if something truly went wrong, not if the item did not exist.
func (dm *DownloadManager) Remove(uuid string) (bool, error) {
	if err := dm.storage.DeleteDownloadState(dm.ctx, uuid); err != nil {
		return false, err
	}

	var handle *DownloadHandle
	dm.mu.Lock()
	for i, hdl := range dm.handles {
		if hdl.uuid == uuid {
			handle = hdl
			dm.handles = slices.Concat(dm.handles[:i], dm.handles[i+1:])
			break
		}
	}
	dm.mu.Unlock()

	if handle != nil {
		stopFnPtr := handle.stopFnOrNil.Load()
		if stopFnPtr != nil {
			(*stopFnPtr)(pb.DownloadStatus_DOWNLOAD_STATUS_CANCELED)
		}

		pub := dm.eventBus.CreatePublisher(&v1.EventContext{
			ServerUuid: handle.server.Uuid,
		})
		pub.Publish(&v1.Event{
			Type: v1.Event_TYPE_DM_ITEM_REMOVED,
			DmItemRemoved: &v1.Event_DmItemRemoved{
				Uuid: uuid,
			},
		})

		return true, nil
	}

	return false, nil
}

// StopWithStatus stops the handle with the specified UUID and sets its status.
// Returns true if the handle was found, returns false otherwise.
func (dm *DownloadManager) StopWithStatus(uuid string, status pb.DownloadStatus) bool {
	handle, has := dm.getByUuid(uuid)
	if !has {
		return false
	}

	fnPtr := handle.stopFnOrNil.Load()

	if fnPtr != nil {
		(*fnPtr)(status)
	}

	return true
}

// DownloadNow starts or resumes the download for the item with the specified UUID.
// Returns true if the item existed, or false otherwise.
// The download is launched and managed in the background.
func (dm *DownloadManager) DownloadNow(uuid string) bool {
	handle, has := dm.getByUuid(uuid)
	if !has {
		return false
	}

	go func() {
		if err := dm.startDownload(handle); err != nil {
			dm.logger.Error("failed to do download",
				"service", "client.DownloadManager",
				"uuid", uuid,
				"error", err,
			)
		}
	}()

	return true
}

func (dm *DownloadManager) mkIncompletePath(serverUuid string, peerUsername common.NormalizedUsername, path common.ProtoPath) string {
	return filepath.Join(
		dm.dirIncomplete,
		dm.incompleteFnReplacer.ReplacePath(filepath.Join(peerUsername.String()+"-"+serverUuid, path.String())),
	)
}
func (dm *DownloadManager) mkCompletePath(serverUuid string, peerUsername common.NormalizedUsername, path common.ProtoPath) string {
	return filepath.Join(
		dm.dirComplete,
		dm.completeFnReplacer.ReplacePath(filepath.Join(peerUsername.String()+"-"+serverUuid, path.String())),
	)
}

func (dm *DownloadManager) trySendUpdate(update dmUpdate) {
	select {
	case dm.pendingUpdates <- update:
	default:
	}
}

func (dm *DownloadManager) startDownload(handle *DownloadHandle) error {
	dm.activeWorkers.Add(1)
	defer dm.activeWorkers.Add(-1)

	// Return immediately if the file is in the pending or done status.
	{
		status := *handle.status.Load()
		if status == pb.DownloadStatus_DOWNLOAD_STATUS_PENDING || status == pb.DownloadStatus_DOWNLOAD_STATUS_DONE {
			return nil
		}
	}

	handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_PENDING))

	// Create paths.
	incompletePath := dm.mkIncompletePath(handle.server.Uuid, handle.peer, handle.filePath)
	completePath := dm.mkCompletePath(handle.server.Uuid, handle.peer, handle.filePath)
	dir := filepath.Dir(incompletePath)
	mkErr := os.MkdirAll(dir, 0755)
	if mkErr != nil {
		return fmt.Errorf(`failed to create directory %q for incomplete download: %w`, dir, mkErr)
	}
	dir = filepath.Dir(completePath)
	mkErr = os.MkdirAll(dir, 0755)
	if mkErr != nil {
		return fmt.Errorf(`failed to create directory %q for complete download: %w`, dir, mkErr)
	}

	// Use TryDo because we want to fail fast if there is not an open connection.
	finalErr := handle.server.TryDo(func(conn *room.Conn) error {
		peer := conn.GetVirtualC2cConn(handle.peer, false)

		initialDownloaded := handle.fileDownloadedBytes.Load()

		meta, reader, err := peer.GetFile(&pb.MsgGetFile{
			Path:   handle.filePath.String(),
			Offset: initialDownloaded,
		})
		if err != nil {
			if protoErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
				if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
					return err
				}
			}

			return err
		}
		defer func() {
			_ = reader.Close()
		}()

		if meta.IsDir {
			// Crawl and queue directory contents in background.
			go func() {
				walkErr := WalkPeerPath(peer, handle.filePath, func(path common.ProtoPath, meta *pb.MsgFileMeta) bool {
					if meta.IsDir {
						return true
					}

					queueErr := dm.Queue(handle.server, handle.peer, path)
					if queueErr != nil {
						dm.logger.Error("failed to queue file while walking directory",
							"service", "client.DownloadManager",
							"server_uuid", handle.server.Uuid,
							"peer_username", handle.peer.String(),
							"dir_path", handle.filePath.String(),
							"file_path", path.String(),
							"error", queueErr,
						)
						return false
					}

					return true
				})
				if walkErr != nil {
					dm.logger.Error("failed to walk directory contents",
						"service", "client.DownloadManager",
						"server_uuid", handle.server.Uuid,
						"peer_username", handle.peer.String(),
						"path", handle.filePath.String(),
						"error", walkErr,
					)
				}
			}()

			return errIsDir
		}

		var fileTotalSize uint64
		if loaded := handle.fileTotalSize.Load(); loaded > -1 {
			fileTotalSize = uint64(loaded)
		} else {
			handle.fileTotalSize.Store(int64(meta.Size))
			fileTotalSize = meta.Size
		}

		if meta.Size != fileTotalSize {
			return errors.New("file size different; file has changed")
		}

		// We have a working stream.
		// Open file.
		file, err := os.OpenFile(incompletePath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf(`failed to open file %q for pending download: %w`, incompletePath, err)
		}
		defer func() {
			_ = file.Close()
		}()

		// If necessary, seek in the file to the current progress.
		if initialDownloaded > 0 {
			_, err = file.Seek(int64(initialDownloaded), io.SeekStart)
			if err != nil {
				return fmt.Errorf(`failed to seek in file %q to byte %d to resume pending download: %w`, incompletePath, initialDownloaded, err)
			}
		}

		ctx, cancel := context.WithCancel(dm.ctx)
		defer cancel()

		// Dump statistics in event channel every second.
		go func() {
			ticker := time.NewTicker(1 * time.Second)

			lastBytes := initialDownloaded

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					newBytes := handle.fileDownloadedBytes.Load()
					speed := newBytes - lastBytes

					dm.trySendUpdate(dmUpdate{
						rpc: &v1.DownloadStatusUpdate{
							Uuid:         handle.uuid,
							Status:       v1.DownloadStatus_DOWNLOAD_STATUS_PENDING,
							Downloaded:   newBytes,
							FileSize:     int64(meta.Size),
							Speed:        speed,
							ErrorMessage: nil,
						},
						ds: handle,
					})

					lastBytes = newBytes
				}
			}
		}()

		endChan := make(chan error, 2)
		shouldDl := true

		// Set stopper function.
		handle.stopFnOrNil.Store(new(func(status pb.DownloadStatus) {
			if !shouldDl {
				return
			}
			endChan <- errHandleStopped
			shouldDl = false
			handle.stopFnOrNil.Store(nil)
			handle.status.Store(&status)
		}))

		go func() {
			endChan <- func() error {
				buf := make([]byte, 512*1024)
				for shouldDl {
					var n int
					n, err = reader.Read(buf)
					handle.fileDownloadedBytes.Store(handle.fileDownloadedBytes.Load() + uint64(n))
					isEof := errors.Is(err, io.EOF)
					if err != nil && !isEof {
						return fmt.Errorf(`failed to read from peer %q to file %q: %w`, handle.peer.String(), incompletePath, err)
					}
					if _, err = file.Write(buf[:n]); err != nil {
						return fmt.Errorf(`failed to write to file %q: %w`, incompletePath, err)
					}
					if isEof {
						break
					}
				}
				return nil
			}()
		}()

		return <-endChan
	})

	fileTotalSize := handle.fileTotalSize.Load()
	finalBytes := handle.fileDownloadedBytes.Load()

	trySendUpdate := func(status v1.DownloadStatus, errMsg *string) {
		dm.trySendUpdate(dmUpdate{
			rpc: &v1.DownloadStatusUpdate{
				Uuid:         handle.uuid,
				Status:       status,
				Downloaded:   finalBytes,
				FileSize:     fileTotalSize,
				Speed:        0,
				ErrorMessage: errMsg,
			},
			ds: handle,
		})
	}

	// If no error, set error if final size is not expected.
	if finalErr == nil && finalBytes != uint64(fileTotalSize) {
		// Final downloaded size did not match the total size.
		// Before setting the error, delete the pending file.

		_ = os.Remove(incompletePath)

		finalErr = fmt.Errorf(`finished downloading file %q from peer %q on server %q but its final size was %d/%d bytes`,
			handle.filePath.String(),
			handle.peer.String(),
			handle.server.Uuid,
			finalBytes,
			fileTotalSize,
		)
	}

	// If no error, move file to final destination and set error if failed.
	if finalErr == nil {
		finalErr = os.Rename(incompletePath, completePath)
	}

	// Check error.
	if finalErr != nil {
		if errors.Is(finalErr, errIsDir) {
			// The handle was already removed.
			// Remove handle, since we can't download directories themselves.
			if _, err := dm.Remove(handle.uuid); err != nil {
				dm.logger.Error("failed to remove handle after directory crawl",
					"service", "client.DownloadManager",
					"uuid", handle.uuid,
					"error", err,
				)
			}
			return nil
		}
		if errors.Is(finalErr, errHandleStopped) {
			// DownloadHandle stop function was called.
			// It already set the status, so we do not need to set it.
			trySendUpdate(v1.DownloadStatus(*handle.status.Load()), nil)
			return nil
		}
		if errors.Is(finalErr, ErrConnNotOpen) {
			// Conn not open; queue again.
			handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
			trySendUpdate(v1.DownloadStatus_DOWNLOAD_STATUS_QUEUED, nil)
			return nil
		}
		if errors.Is(finalErr, protocol.ErrPeerUnreachable) {
			// Peer unreachable; queue again.
			handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
			trySendUpdate(v1.DownloadStatus_DOWNLOAD_STATUS_QUEUED, nil)
			return nil
		}
		if protoErr, ok := errors.AsType[protocol.ProtoMsgError](finalErr); ok && protoErr.Msg.Type == pb.ErrType_ERR_TYPE_CLIENT_NOT_ONLINE {
			// Peer unreachable; queue again.
			handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
			trySendUpdate(v1.DownloadStatus_DOWNLOAD_STATUS_QUEUED, nil)
			return nil
		}
		if protocol.IsErrorConnCloseOrCancel(finalErr) || errors.Is(finalErr, ErrConnNannyClosed) {
			// Server connection closed, or application is closed; queue again.
			handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
			trySendUpdate(v1.DownloadStatus_DOWNLOAD_STATUS_QUEUED, nil)
			return nil
		}

		errMsg := finalErr.Error()
		handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_ERROR))
		handle.errorMessage.Store(&errMsg)
		trySendUpdate(v1.DownloadStatus_DOWNLOAD_STATUS_ERROR, &errMsg)
		return finalErr
	}

	// If we got this far, the download completed successfully.
	handle.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_DONE))
	trySendUpdate(v1.DownloadStatus_DOWNLOAD_STATUS_DONE, nil)

	return nil
}
