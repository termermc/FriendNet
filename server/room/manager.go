package room

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"friendnet.org/common"
	"friendnet.org/server/storage"
)

var ErrManagerClosed = fmt.Errorf("room manager is closed")

// Manager manages rooms.
// It is responsible for coordinating room fetching, creation and deletion.
type Manager struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	storage *storage.Storage

	// Key is the string value of a common.NormalizedRoomName.
	rooms map[string]*Room
}

// NewManager creates a new room manager.
// It loads all rooms from storage.
func NewManager(ctx context.Context, logger *slog.Logger, storage *storage.Storage) (*Manager, error) {
	m := &Manager{
		logger:  logger,
		storage: storage,
		rooms:   make(map[string]*Room),
	}

	// Load rooms from storage.
	rooms, err := storage.GetRooms(ctx)
	if err != nil {
		return nil, fmt.Errorf(`failed to get all rooms while creating new room manager: %w`, err)
	}
	for _, room := range rooms {
		m.rooms[room.Name.String()] = NewRoom(logger, room.Name)
	}

	return m, nil
}

// Close closes all rooms and then closes the manager itself.
// Manager must never be used after calling Manager.Close.
// Will never return an error.
// Additional calls are no-op.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return nil
	}
	m.isClosed = true

	// Close all rooms.
	var wg sync.WaitGroup
	for _, room := range m.rooms {
		wg.Go(func() {
			_ = room.Close()
		})
	}

	return nil
}

// CreateRoom creates a new room and returns it.
func (m *Manager) CreateRoom(ctx context.Context, name common.NormalizedRoomName) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return nil, ErrManagerClosed
	}

	// Create room in storage.
	err := m.storage.CreateRoom(ctx, name)
	if err != nil {
		return nil, err
	}

	// Create room instance and add it to manager.
	room := NewRoom(m.logger, name)
	m.rooms[name.String()] = room

	return room, nil
}

// GetRoomByName returns the room with the specified name, if any.
// Returns false if the room does not exist.
func (m *Manager) GetRoomByName(name common.NormalizedRoomName) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	room, ok := m.rooms[name.String()]
	return room, ok
}

// DeleteRoomByName deletes the room with the specified name.
// If the room does not exist, this is a no-op.
func (m *Manager) DeleteRoomByName(ctx context.Context, name common.NormalizedRoomName) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return ErrManagerClosed
	}

	room, has := m.rooms[name.String()]
	if !has {
		return nil
	}

	// Close room first.
	_ = room.Close()

	// Delete from storage.
	return m.storage.DeleteRoomByName(ctx, name)
}
