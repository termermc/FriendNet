package client

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"friendnet.org/client/event"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

const dmDirIncompleteSetting = "dm_dir_incomplete"
const dmDirCompleteSetting = "dm_dir_complete"
const dmDlConcurrencySetting = "dm_dl_concurrency"

// DownloadState is the state of a download.
type DownloadState struct {
	dm *DownloadManager

	status pb.DownloadStatus

	server              *Server
	peer                common.NormalizedUsername
	filePath            common.ProtoPath
	fileTotalSize       int64
	fileDownloadedBytes atomic.Int64
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
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf(`failed to create directory %q for pending download: %w`, dir, err)
	}

	// TODO Try for a connection. Do not wait for one. Use TryDo or whatever the equivalent method on the ConnNanny is.
	return nil
}
