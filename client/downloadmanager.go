package client

import (
	"context"
	"log/slog"
	"sync"

	"friendnet.org/client/event"
	"friendnet.org/client/storage"
)

const dmDirIncompleteSetting = "dm_dir_incomplete"
const dmDirCompleteSetting = "dm_dir_complete"

// DownloadState is the state of a download.
type DownloadState struct {
	dm *DownloadManager
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

	// TODO Queue of status updates.
	// The queue will be drained to a pool of open status update bidis (or maybe just one).
	// If the queue chan is full, status updates will be dropped.
}

func NewDownloadManager(
	logger *slog.Logger,

	multi *MultiClient,
	eventBus *event.Bus,
	storage *storage.Storage,
) (*DownloadManager, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &DownloadManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		logger:    logger,
		multi:     multi,
		eventBus:  eventBus,
		storage:   storage,
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
