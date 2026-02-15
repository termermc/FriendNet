package share

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"friendnet.org/client/storage"
)

// ErrServerManagerClosed is returned by ServerShareManager methods when it is closed.
var ErrServerManagerClosed = errors.New("server manager is closed")

// ServerShareManager manages shares for a server.
type ServerShareManager struct {
	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	serverUuid string
	storage    storage.Storage

	// A mapping of share names to their underlying Share instances.
	shareMap map[string]Share
}

// NewManager creates a new share manager for the given server.
// It gets share records for the server and instantiates Share instances for them.
func NewManager(serverUuid string, storage storage.Storage) (*ServerShareManager, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	// Get shares for server.
	records, err := storage.GetSharesByServer(ctx, serverUuid)
	if err != nil {
		ctxCancel()
		return nil, fmt.Errorf(`failed to get share records for server %q: %w`, serverUuid, err)
	}

	shareMap := make(map[string]Share, len(records))
	for _, record := range records {
		share := NewFsShare(record.Name, os.DirFS(record.Path))
		shareMap[record.Name] = share
	}

	return &ServerShareManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		serverUuid: serverUuid,
		storage:    storage,

		shareMap: shareMap,
	}, nil
}

func (m *ServerShareManager) snapshotShares() []Share {
	slice := make([]Share, 0, len(m.shareMap))
	for _, share := range m.shareMap {
		slice = append(slice, share)
	}
	return slice
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

	return m.snapshotShares()
}

// TODO Get, Add, Delete methods.

// Close closes all shares managed by the manager, then the manager itself.
func (m *ServerShareManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return nil
	}
	m.isClosed = true

	m.ctxCancel()

	// Close all shares.
	var wg sync.WaitGroup
	for _, share := range m.shareMap {
		wg.Go(func() {
			_ = share.Close()
		})
	}
	wg.Wait()

	return nil
}
