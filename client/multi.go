package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"friendnet.org/client/cert"
	"friendnet.org/client/room"
	"friendnet.org/client/share"
	"friendnet.org/client/storage"
	"friendnet.org/common"
)

// ErrMultiClientClosed is returned by MultiClient methods when the MultiClient is closed.
var ErrMultiClientClosed = errors.New("multi client is closed")

// Server includes state for managing a server connection.
type Server struct {
	// The server UUID.
	// Do not update.
	Uuid string

	// The name set for the server.
	// Do not update.
	Name string

	// The server record's creation timestamp.
	CreatedTs time.Time

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

	c := &MultiClient{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		logger:    logger,
		storage:   storage,
		certStore: certStore,
		servers:   make(map[string]Server, len(serverRecs)),
	}

	for _, record := range serverRecs {
		var inst Server
		inst, err = c.createServerInstance(record)
		if err != nil {
			ctxCancel()
			return nil, err
		}

		c.servers[record.Uuid] = inst
	}

	return c, nil
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

func (c *MultiClient) createServerInstance(record storage.ServerRecord) (Server, error) {
	var shareMgr *share.ServerShareManager
	shareMgr, err := share.NewServerShareManager(
		record.Uuid,
		c.storage,
	)
	if err != nil {
		return Server{}, err
	}

	logic := room.NewLogicImpl(shareMgr)

	return Server{
		Uuid:      record.Uuid,
		Name:      record.Name,
		CreatedTs: record.CreatedTs,
		ConnNanny: NewConnNanny(
			c.logger,
			c.certStore,
			record.Address,
			room.Credentials{
				Room:     record.Room,
				Username: record.Username,
				Password: record.Password,
			},
			logic,
		),
	}, nil
}

// Create creates a new server record in storage and starts managing a connection to it.
func (c *MultiClient) Create(
	ctx context.Context,
	name string,
	address string,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) (Server, error) {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return Server{}, ErrMultiClientClosed
	}
	c.mu.Unlock()

	uuid, err := c.storage.CreateServer(
		ctx,
		name,
		address,
		room,
		username,
		password,
	)
	if err != nil {
		return Server{}, fmt.Errorf(`failed to create server %q in storage: %w`, name, err)
	}

	ok := false
	defer func() {
		if !ok {
			_ = c.storage.DeleteServerByUuid(ctx, uuid)
		}
	}()

	// Return record.
	record, err := c.storage.GetServerByUuid(ctx, uuid)
	if err != nil {
		return Server{}, fmt.Errorf(`failed to get server record for server %q (UUID: %q): %w`, name, uuid, err)
	}

	inst, err := c.createServerInstance(record)
	if err != nil {
		return Server{}, fmt.Errorf(`failed to create server instance for server %q (UUID: %q): %w`, name, uuid, err)
	}

	c.mu.Lock()
	c.servers[uuid] = inst
	c.mu.Unlock()

	ok = true

	return inst, nil
}

// Update updates a server's record in storage and in memory.
// It does not interrupt any connections, and any changes to the connection parameters will take effect on the next reconnect.
func (c *MultiClient) Update(
	ctx context.Context,
	uuid string,
	name *string,
	address *string,
	room *common.NormalizedRoomName,
	username *common.NormalizedUsername,
	password *string,
) error {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return ErrMultiClientClosed
	}

	server, hasServer := c.servers[uuid]
	c.mu.Unlock()

	// Update in storage.
	err := c.storage.UpdateServer(
		ctx,
		uuid,
		name,
		address,
		room,
		username,
		password,
	)
	if err != nil {
		return fmt.Errorf(`failed to update server UUID %q in storage: %w`, uuid, err)
	}

	// Update in memory.
	if hasServer {
		if name != nil {
			server.Name = *name
		}
		if address != nil {
			server.SetAddress(*address)
		}
		if room != nil {
			server.SetRoom(*room)
		}
		if username != nil {
			server.SetUsername(*username)
		}
		if password != nil {
			server.SetPassword(*password)
		}
	}

	return nil
}

// DeleteByUuid deletes the server record from storage and closes its connection, if any.
// If the server does not exist, this is a no-op.
func (c *MultiClient) DeleteByUuid(
	ctx context.Context,
	uuid string,
) error {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return ErrMultiClientClosed
	}

	conn, hasConn := c.servers[uuid]
	if hasConn {
		delete(c.servers, uuid)
	}
	c.mu.Unlock()

	// Delete server in storage.
	// We do this without checking hasConn because it may still exist in storage even if not in memory.
	err := c.storage.DeleteServerByUuid(ctx, uuid)
	if err != nil {
		return fmt.Errorf(`failed to delete server %q from storage: %w`, uuid, err)
	}

	if hasConn {
		_ = conn.Close()
	}

	return nil
}
