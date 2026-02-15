package client

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"friendnet.org/client/cert"
	"friendnet.org/client/room"
	"friendnet.org/client/share"
	"friendnet.org/client/storage"
)

// ErrMultiClientClosed is returned by MultiClient methods when the MultiClient is closed.
var ErrMultiClientClosed = errors.New("multi client is closed")

// Server includes state for managing a server connection.
type Server struct {
	// The server UUID.
	// Do not update.
	Uuid string

	*ConnNanny
}

// MultiClient is a FriendNet client that manages multiple room connections.
// It can create and tear down connections within its lifecycle, and manages higher-level components like shares
// independent of connections.
type MultiClient struct {
	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	logger    *slog.Logger
	storage   storage.Storage
	certStore cert.Store

	// Mapping of server UUIDs to the Server instances that manage connections to them.
	servers map[string]Server
}

// NewMultiClient creates a new MultiClient instance.
// It loads all room data from storage and starts managing connections to them.
func NewMultiClient(
	logger *slog.Logger,
	storage storage.Storage,
	certStore cert.Store,
) (*MultiClient, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	serverRecs, err := storage.GetServers(ctx)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	servers := make(map[string]Server, len(serverRecs))
	for _, record := range serverRecs {
		var shareMgr *share.ServerShareManager
		shareMgr, err = share.NewServerShareManager(
			record.Uuid,
			storage,
		)
		if err != nil {
			ctxCancel()
			return nil, err
		}

		logic := room.NewLogicImpl(shareMgr)

		servers[record.Uuid] = Server{
			Uuid: record.Uuid,
			ConnNanny: NewConnNanny(
				logger,
				certStore,
				record.Address,
				room.Credentials{
					Room:     record.Room,
					Username: record.Username,
					Password: record.Password,
				},
				logic,
			),
		}
	}

	return &MultiClient{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		logger:    logger,
		storage:   storage,
		certStore: certStore,
		servers:   servers,
	}, nil
}

func (c *MultiClient) snapshotServers() []Server {
	slice := make([]Server, 0, len(c.servers))
	for _, server := range c.servers {
		slice = append(slice, server)
	}
	return slice
}

// Close closes all connections managed by the MultiClient, and the MultiClient itself.
func (c *MultiClient) Close() error {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return nil
	}
	c.isClosed = true

	rooms := c.snapshotServers()
	c.mu.Unlock()

	// Close all connections.
	var wg sync.WaitGroup
	for _, cn := range rooms {
		wg.Go(func() {
			_ = cn.Close()
		})
	}
	wg.Wait()

	return nil
}

// GetAll returns all server connections under management.
// Returns an empty slice if the MultiClient is closed.
// Note that this method creates a new slice each time it is called.
func (c *MultiClient) GetAll() []Server {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.isClosed {
		return nil
	}

	return c.snapshotServers()
}

// GetByUuid returns the server connection for the server with the specified UUID and true if found,
// otherwise empty and false.
// Returns empty and false if the MultiClient is closed.
func (c *MultiClient) GetByUuid(uuid string) (Server, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.isClosed {
		return Server{}, false
	}

	server, has := c.servers[uuid]
	return server, has
}
