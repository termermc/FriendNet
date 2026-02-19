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

// ErrShareExists is returned when trying to create a new share with a name that already exists.
var ErrShareExists = errors.New("share with same name exists")

// ServerShareManager manages shares for a server.
type ServerShareManager struct {
	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	serverUuid string
	storage    *storage.Storage

	// A mapping of share names to their underlying Share instances.
	shareMap map[string]Share
}

// NewServerShareManager creates a new share manager for the given server.
// It gets share records for the server and instantiates Share instances for them.
func NewServerShareManager(serverUuid string, storage *storage.Storage) (*ServerShareManager, error) {
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

// GetByName returns the share with the specified name and true, or nil and false if no such share name exists.
// Always returns nil and false if the manager is closed.
func (m *ServerShareManager) GetByName(name string) (Share, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil, false
	}

	share, has := m.shareMap[name]
	return share, has
}

// Add creates a new server share.
// If a share with the same name exists, returns ErrShareExists.
func (m *ServerShareManager) Add(ctx context.Context, name string, path string) (Share, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return nil, ErrServerManagerClosed
	}

	_, exists := m.shareMap[name]
	if exists {
		return nil, ErrShareExists
	}

	// Create in storage.
	err := m.storage.CreateShare(ctx, m.serverUuid, name, path)
	if err != nil {
		return nil, fmt.Errorf(`failed to create new share %q: %w`, name, err)
	}

	// Create instance.
	share := NewFsShare(name, os.DirFS(path))
	m.shareMap[name] = share

	return share, nil
}

// Delete deletes an existing server share.
// If the share does not exist, this is no-op.
func (m *ServerShareManager) Delete(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return ErrShareClosed
	}

	share, has := m.shareMap[name]
	if !has {
		return nil
	}

	// Remove from storage.
	err := m.storage.DeleteShareByServerAndName(ctx, m.serverUuid, name)
	if err != nil {
		return fmt.Errorf(`failed to remove share with server UUID %q and name %q: %w`, m.serverUuid, name, err)
	}

	// Close share and remove it from map.
	_ = share.Close()
	delete(m.shareMap, name)

	return nil
}

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
