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
	"sync"
	"sync/atomic"
	"time"

	"friendnet.org/client/event"
	"friendnet.org/client/room"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/protocol"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	pb "friendnet.org/protocol/pb/v1"
)

const dmDirIncompleteSetting = "dm_dir_incomplete"
const dmDirCompleteSetting = "dm_dir_complete"
const dmDlConcurrencySetting = "dm_dl_concurrency"

type dmUpdate struct {
	rpc *v1.DownloadStatusUpdate
	ds  *DownloadState
}

func (u *dmUpdate) ToProto() *pb.MsgDownloadStatusUpdate {
	return &pb.MsgDownloadStatusUpdate{
		Path: u.ds.filePath.String(),

		// Enum is duplicate and can be casted directly.
		Status: pb.DownloadStatus(u.rpc.Status),

		BytesDownloaded: u.rpc.Downloaded,
	}
}

// DownloadState is the state of a download.
type DownloadState struct {
	dm *DownloadManager

	status atomic.Pointer[pb.DownloadStatus]

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
	fileTotalSize int64

	// The file's current download progress.
	fileDownloadedBytes atomic.Uint64
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

	logger *slog.Logger

	multi    *MultiClient
	eventBus *event.Bus
	storage  *storage.Storage

	dirIncomplete string
	dirComplete   string
	dlConcurrency int

	states []*DownloadState

	// A queue of pending download progress events to send to the event bus.
	// It is buffered, but sends should be discarded if the buffer is full instead of blocking.
	pendingUpdates chan dmUpdate
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

	defDlBaseDir := filepath.Join(homeDir, "FriendNet Downloads")
	defDlIncomplete := filepath.Join(defDlBaseDir, "Incomplete")
	defDlComplete := filepath.Join(defDlBaseDir, "Complete")

	// Get settings.
	dirIncomplete, err := storage.GetSettingOrPut(ctx, dmDirIncompleteSetting, defDlIncomplete)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	dirComplete, err := storage.GetSettingOrPut(ctx, dmDirCompleteSetting, defDlComplete)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	dlConcurrency, err := storage.GetSettingIntOrPut(ctx, dmDlConcurrencySetting, 4)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	return &DownloadManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		logger: logger,

		multi:    multi,
		eventBus: eventBus,
		storage:  storage,

		dirIncomplete: dirIncomplete,
		dirComplete:   dirComplete,
		dlConcurrency: int(dlConcurrency),
	}, nil
}

func (dm *DownloadManager) eventDrainer() {

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

func (dm *DownloadManager) mkPath(serverUuid string, peerUsername common.NormalizedUsername, path common.ProtoPath) string {
	return filepath.Join(dm.dirComplete, peerUsername.String()+"-"+serverUuid, path.String())
}

func (dm *DownloadManager) trySendUpdate(update dmUpdate) {
	select {
	case dm.pendingUpdates <- update:
	default:
	}
}

func (dm *DownloadManager) startDownload(state *DownloadState) error {
	// Create path.
	pendingPath := dm.mkPath(state.server.Uuid, state.peer, state.filePath)
	dir := filepath.Dir(pendingPath)
	mkErr := os.MkdirAll(dir, 0755)
	if mkErr != nil {
		return fmt.Errorf(`failed to create directory %q for pending download: %w`, dir, mkErr)
	}

	// Use TryDo because we want to fail fast if there is not an open connection.
	finalErr := state.server.TryDo(func(conn *room.Conn) error {
		peer := conn.GetVirtualC2cConn(state.peer, false)

		initialDownloaded := state.fileDownloadedBytes.Load()

		meta, reader, err := peer.GetFile(&pb.MsgGetFile{
			Path:   state.filePath.String(),
			Offset: initialDownloaded,
		})
		if err != nil {
			if protoErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
				if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
					// TODO Specific error when file does not exist anymore.
					// TODO Possibly same error that is returned when the size is different.
					return err
				}
			}

			return err
		}

		if meta.Size != uint64(state.fileTotalSize) {
			// TODO Figure out a good way to signal that the file has changed.
			return errors.New("file size different; file has changed")
		}

		// We have a working stream.
		// Open file.
		file, err := os.OpenFile(pendingPath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf(`failed to open file %q for pending download: %w`, pendingPath, err)
		}
		defer func() {
			_ = file.Close()
		}()

		// If necessary, seek in the file to the current progress.
		if initialDownloaded > 0 {
			_, err = file.Seek(int64(initialDownloaded), io.SeekStart)
			if err != nil {
				return fmt.Errorf(`failed to seek in file %q to byte %d to resume pending download: %w`, pendingPath, initialDownloaded, err)
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
					newBytes := state.fileDownloadedBytes.Load()
					speed := newBytes - lastBytes

					dm.trySendUpdate(dmUpdate{
						rpc: &v1.DownloadStatusUpdate{
							Uuid:         state.uuid,
							Status:       v1.DownloadStatus_DOWNLOAD_STATUS_PENDING,
							Downloaded:   newBytes,
							FileSize:     meta.Size,
							Speed:        speed,
							ErrorMessage: nil,
						},
						ds: state,
					})
				}
			}
		}()

		buf := make([]byte, 512*1024)
		for {
			var n int
			n, err = reader.Read(buf)
			state.fileDownloadedBytes.Store(state.fileDownloadedBytes.Load() + uint64(n))
			isEof := errors.Is(err, io.EOF)
			if err != nil && !isEof {
				return fmt.Errorf(`failed to read from peer %q to file %q: %w`, state.peer.String(), pendingPath, err)
			}
			if _, err = file.Write(buf[:n]); err != nil {
				return fmt.Errorf(`failed to write to file %q: %w`, pendingPath, err)
			}
			if isEof {
				break
			}
		}

		return nil
	})

	finalBytes := state.fileDownloadedBytes.Load()
	if finalErr == nil && finalBytes != uint64(state.fileTotalSize) {
		// Final downloaded size did not match the total size.
		// Before setting the error, delete the pending file.

		_ = os.Remove(pendingPath)

		finalErr = fmt.Errorf(`finished downloading file %q from peer %q on server %q but its final size was %d/%d bytes`,
			state.filePath.String(),
			state.peer.String(),
			state.server.Uuid,
			finalBytes,
			state.fileTotalSize,
		)
	}

	if finalErr != nil {
		if errors.Is(finalErr, ErrConnNotOpen) {
			// Conn not open; queue again.
			state.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
			return nil
		}
		if errors.Is(finalErr, protocol.ErrPeerUnreachable) {
			// Peer unreachable; queue again.
			state.status.Store(new(pb.DownloadStatus_DOWNLOAD_STATUS_QUEUED))
			return nil
		}

		dm.trySendUpdate(dmUpdate{
			rpc: &v1.DownloadStatusUpdate{
				Uuid:         state.uuid,
				Status:       v1.DownloadStatus_DOWNLOAD_STATUS_ERROR,
				Downloaded:   state.fileDownloadedBytes.Load(),
				FileSize:     uint64(state.fileTotalSize),
				Speed:        0,
				ErrorMessage: new(finalErr.Error()),
			},
			ds: state,
		})
		return finalErr
	}

	// TODO Move file to final destination.

	dm.trySendUpdate(dmUpdate{
		rpc: &v1.DownloadStatusUpdate{
			Uuid:         state.uuid,
			Status:       v1.DownloadStatus_DOWNLOAD_STATUS_DONE,
			Downloaded:   state.fileDownloadedBytes.Load(),
			FileSize:     uint64(state.fileTotalSize),
			Speed:        0,
			ErrorMessage: nil,
		},
		ds: state,
	})

	return nil
}
