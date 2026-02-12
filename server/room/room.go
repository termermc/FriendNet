package room

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

var ErrRoomClosed = errors.New("room closed")
var ErrUsernameAlreadyConnected = errors.New("client with same username already connected to room")

// TODO Protocol message for server-driven close and reason.

// Room is a server room that manages connected clients.
type Room struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	// The room's name.
	Name common.NormalizedRoomName

	// The room's context.
	// Canceled when it is closed.
	Context context.Context

	ctxCancel             context.CancelFunc
	clientMessageHandlers ClientMessageHandlers
	// Key is the string value of a common.NormalizedUsername.
	clients map[string]*Client
}

// NewRoom creates a new room instance.
// The room manages clients within it.
func NewRoom(
	logger *slog.Logger,
	name common.NormalizedRoomName,
	clientMessageHandlers ClientMessageHandlers,
) *Room {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &Room{
		logger:                logger,
		Name:                  name,
		Context:               ctx,
		ctxCancel:             ctxCancel,
		clientMessageHandlers: clientMessageHandlers,
		clients:               make(map[string]*Client),
	}
}

// snapshotClients snapshots all clients currently connected and returns a slice of them.
// Note that this method calls RLock.
// It is best to call this, then do work on the clients without locking.
func (r *Room) snapshotClients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

// Close closes all client connections in the room and then closes the room itself.
// Room.Onboard must not be called after Close.
// Will never return an error.
func (r *Room) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isClosed {
		return nil
	}

	r.isClosed = true

	// Close all client connections.
	var wg sync.WaitGroup
	for _, client := range r.clients {
		wg.Go(func() {
			_ = client.conn.CloseWithReason("room closed")
		})
	}
	wg.Wait()

	r.clients = nil

	return nil
}

// Onboard takes ownership of a connection and adds it to the room.
// The connection must already have been authenticated.
//
// If there is an existing client with the username, returns ErrUsernameAlreadyConnected.
// This method will not close the connection if it returns an error; it is the caller's responsibility to close it if an error is returned.
func (r *Room) Onboard(
	conn protocol.ProtoConn,
	version *pb.ProtoVersion,
	username common.NormalizedUsername,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed {
		return ErrRoomClosed
	}

	client := NewClient(
		r.logger,
		conn,
		version,
		r,
		username,
		r.clientMessageHandlers,
	)

	_, has := r.clients[username.String()]
	if has {
		return ErrUsernameAlreadyConnected
	}

	r.clients[username.String()] = client

	// Ping loop.
	go func() {
		defer func() {
			if err := recover(); err != nil {
				r.logger.Error("client ping loop panicked",
					"service", "room.Client",
					"room", r.Name.String(),
					"username", username.String(),
					slog.Any("err", err),
				)
			}
		}()

		client.PingLoop(r.Context)
	}()

	// TODO Read loop
	// If read loop exits with an error, call some method to do client disconnection and cleanup.

	// TODO Broadcast online state

	return nil
}

// GetClientByUsername returns the client with the specified username, if any.
// The bool value is whether there was a client with that username.
// Always returns false if the room is closed.
func (r *Room) GetClientByUsername(username common.NormalizedUsername) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.isClosed {
		return nil, false
	}

	client, has := r.clients[username.String()]
	return client, has
}
