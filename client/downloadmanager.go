package client

import (
	"context"
	"sync"

	"friendnet.org/client/event"
	"friendnet.org/client/storage"
)

const dmDirIncompleteSetting = "dm_dir_incomplete"
const dmDirCompleteSetting = "dm_dir_complete"

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

	multi    *MultiClient
	eventBus *event.Bus
	storage  *storage.Storage
}
