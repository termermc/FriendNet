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

// DownloadState is the state of a download.
type DownloadState struct {
	dm *DownloadManager

	status pb.DownloadStatus

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

	// A queue of pending download progress events to send to the event bus.
	// It is buffered, but sends should be discarded if the buffer is full instead of blocking.
	pendingEvents chan *v1.DownloadProgress
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

func (dm *DownloadManager) startDownload(state *DownloadState) error {
	// Create path.
	pendingPath := dm.mkPath(state.server.Uuid, state.peer, state.filePath)
	dir := filepath.Dir(pendingPath)
	mkErr := os.MkdirAll(dir, 0755)
	if mkErr != nil {
		return fmt.Errorf(`failed to create directory %q for pending download: %w`, dir, mkErr)
	}

	// Use TryDo because we want to fail fast if there is not an open connection.
	return state.server.TryDo(func(conn *room.Conn) error {
		peer := conn.GetVirtualC2cConn(state.peer, false)

		initialDownloaded := state.fileDownloadedBytes.Load()

		meta, reader, err := peer.GetFile(&pb.MsgGetFile{
			Path:   state.filePath.String(),
			Offset: uint64(initialDownloaded),
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
		// TODO Do we want to be bundling extra data in the events to send out notifications to the peer we're downloading from?
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

					dlEvent := &v1.DownloadProgress{
						Uuid:       state.uuid,
						Downloaded: newBytes,
						Speed:      speed,
					}

					// Try to send to pendingEvents channel, discard if buffer full.
					select {
					case dm.pendingEvents <- dlEvent:
					default:
					}
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

		// TODO If file was read in its entirety, move it to its destination folder and send event.

		return nil
	})
}
