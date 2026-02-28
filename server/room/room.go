package room

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"friendnet.org/common"
	"friendnet.org/common/machine"
	pass "friendnet.org/common/password"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"friendnet.org/server/storage"
	"github.com/quic-go/quic-go"
	mcfpassword "github.com/termermc/go-mcf-password"
	"google.golang.org/protobuf/proto"
)

var ErrRoomClosed = errors.New("room closed")
var ErrUsernameAlreadyConnected = errors.New("client with same username already connected to room")
var ErrAccountExists = errors.New("account with same username already exists")
var ErrNoSuchAccount = errors.New("no such account")

// TODO Protocol message for server-driven close and reason.

// Room is a server room that manages connected clients.
type Room struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	storage           *storage.Storage
	connMethodSupport machine.ConnMethodSupport
	passReqs          pass.Requirements

	// The room's name.
	Name common.NormalizedRoomName

	// The room's token manager.
	TokenManager *TokenManager

	// The room's context.
	// Canceled when it is closed.
	Context   context.Context
	ctxCancel context.CancelFunc

	logic Logic

	// Key is the string value of a common.NormalizedUsername.
	clients map[string]*Client
}

// NewRoom creates a new room instance.
// The room manages clients within it.
func NewRoom(
	logger *slog.Logger,
	storage *storage.Storage,
	connMethodSupport machine.ConnMethodSupport,
	passReqs pass.Requirements,
	name common.NormalizedRoomName,
	logic Logic,
) *Room {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &Room{
		logger: logger,

		storage:           storage,
		connMethodSupport: connMethodSupport,
		passReqs:          passReqs,

		Name: name,

		TokenManager: NewTokenManager(ctx, DefaultTokenValidDuration, DefaultTokenExpiredGcInterval),

		Context:   ctx,
		ctxCancel: ctxCancel,

		logic: logic,

		clients: make(map[string]*Client),
	}
}

func (r *Room) snapshotClientsNoLock() []*Client {
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

	// Signal to the client connections that the server is shutting down.
	// Give them 5 seconds to respond before closing the connections.
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		var byeWg sync.WaitGroup
		for _, client := range r.clients {
			byeWg.Go(func() {
				_, _ = client.conn.SendAndReceive(pb.MsgType_MSG_TYPE_BYE, &pb.MsgBye{})
			})
		}
		byeWg.Wait()
		cancel()
	}()
	<-timeoutCtx.Done()

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

// ClientCount returns the current number of clients.
// Returns 0 if the room is closed.
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.isClosed {
		return 0
	}

	return len(r.clients)
}

// GetAllClients returns all connected clients.
// Returns empty if the room is closed.
// Note that this method creates a new slice each time it is called.
func (r *Room) GetAllClients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.isClosed {
		return nil
	}

	return r.snapshotClientsNoLock()
}

// Broadcast broadcasts a message to all clients in the room.
// It is fire-and-forget and returns quickly, not waiting for the message to be sent.
// No-op if the room is closed.
func (r *Room) Broadcast(typ pb.MsgType, msg proto.Message) {
	go func() {
		clients := r.GetAllClients()
		for _, client := range clients {
			go func() {
				bidi, err := client.conn.OpenBidiWithMsg(typ, msg)
				if err != nil {
					if protocol.IsErrorConnCloseOrCancel(err) {
						return
					}

					r.logger.Error("failed to broadcast message to client",
						"service", "room.Room",
						"username", client.Username.String(),
						"message_type", typ.String(),
					)
				}
				time.Sleep(100 * time.Millisecond)
				_ = bidi.Close()
			}()
		}
	}()
}

// Onboard takes ownership of a connection and adds it to the room.
// The connection must already have been authenticated.
//
// If onboarding is successful, it will write the auth accepted message to authBidi and close it.
//
// If there is an existing client with the username, returns ErrUsernameAlreadyConnected.
// This method will not close the connection if it returns an error; it is the caller's responsibility to close it if an error is returned.
func (r *Room) Onboard(
	authBidi protocol.ProtoBidi,
	conn protocol.ProtoConn,
	version *pb.ProtoVersion,
	username common.NormalizedUsername,
) error {
	r.mu.RLock()
	if r.isClosed {
		r.mu.RUnlock()
		return ErrRoomClosed
	}

	_, has := r.clients[username.String()]
	if has {
		r.mu.RUnlock()
		return ErrUsernameAlreadyConnected
	}

	r.mu.RUnlock()

	client := NewClient(
		r.logger,
		conn,
		version,
		r,
		username,
		r.logic,
	)

	r.mu.Lock()
	r.handleConnect(client)
	r.mu.Unlock()

	err := authBidi.Write(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, &pb.MsgAuthAccepted{})
	if err != nil {
		r.mu.Lock()
		r.handleDisconnect(client)
		r.mu.Unlock()
		return fmt.Errorf("failed to write auth accepted message: %w", err)
	}
	_ = authBidi.Close()

	// Ping loop.
	go func() {
		defer func() {
			if err := recover(); err != nil {
				r.logger.Error("client ping loop panicked",
					"service", "room.Client",
					"room", r.Name.String(),
					"username", username.String(),
					"err", err,
					"stack", string(debug.Stack()),
				)
			}
		}()

		client.PingLoop(r.Context)

		r.mu.Lock()
		r.handleDisconnect(client)
		r.mu.Unlock()
	}()

	// Read loop.
	go func() {
		defer func() {
			if err := recover(); err != nil {
				r.logger.Error("client read loop panicked",
					"service", "room.Client",
					"room", r.Name.String(),
					"username", username.String(),
					"err", err,
					"stack", string(debug.Stack()),
				)
			}
		}()

		if err := client.ReadLoop(r.Context); err != nil {
			var idleErr *quic.IdleTimeoutError
			var appErr *quic.ApplicationError
			if !errors.Is(err, context.Canceled) && !errors.As(err, &idleErr) && !errors.As(err, &appErr) {
				r.logger.Error("client read loop exited with error",
					"service", "room.Room",
					"room", r.Name.String(),
					"username", username.String(),
					"err", err,
				)
			}
		}

		r.mu.Lock()
		r.handleDisconnect(client)
		r.mu.Unlock()
	}()

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

// CreateAccount creates a new account in the room.
// Returns ErrAccountExists if an account with the same username already exists.
// Returns a password.Error if the password does not meet the room's requirements.
func (r *Room) CreateAccount(ctx context.Context, username common.NormalizedUsername, password string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isClosed {
		return ErrRoomClosed
	}

	_, has, err := r.storage.GetAccountByRoomAndUsername(ctx, r.Name, username)
	if err != nil {
		return fmt.Errorf(`failed to check if account %q@%q already exists in CreateAccount: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}
	if has {
		return ErrAccountExists
	}

	hash, err := pass.HashWithRequirements(username, password, r.passReqs)
	if err != nil {
		return fmt.Errorf(`failed to hash password for account %q@%q in CreateAccount: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}

	err = r.storage.CreateAccount(ctx, r.Name, username, hash)
	if err != nil {
		return fmt.Errorf(`failed to create account %q@%q in CreateAccount: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}

	return nil
}

// DeleteAccount deletes an account from the room.
// If the account does not exist, returns ErrNoSuchAccount.
func (r *Room) DeleteAccount(ctx context.Context, username common.NormalizedUsername) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isClosed {
		return ErrRoomClosed
	}

	_, has, err := r.storage.GetAccountByRoomAndUsername(ctx, r.Name, username)
	if err != nil {
		return fmt.Errorf(`failed to check if account %q@%q exists in DeleteAccount: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}
	if !has {
		return ErrNoSuchAccount
	}

	err = r.storage.DeleteAccountByRoomAndUsername(ctx, r.Name, username)
	if err != nil {
		return fmt.Errorf(`failed to delete account %q@%q in DeleteAccount: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}

	return nil
}

// UpdateAccountPassword updates the password of an account in the room.
// If the account does not exist, returns ErrNoSuchAccount.
// Returns a password.Error if the password does not meet the room's requirements.
func (r *Room) UpdateAccountPassword(ctx context.Context, username common.NormalizedUsername, password string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isClosed {
		return ErrRoomClosed
	}

	hash, err := pass.HashWithRequirements(username, password, r.passReqs)
	if err != nil {
		return fmt.Errorf(`failed to hash password for account %q@%q in UpdateAccountPassword: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}

	_, has, err := r.storage.GetAccountByRoomAndUsername(ctx, r.Name, username)
	if err != nil {
		return fmt.Errorf(`failed to check if account %q@%q exists in UpdateAccountPassword: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}
	if !has {
		return ErrNoSuchAccount
	}

	err = r.storage.UpdateAccountPasswordHash(ctx, r.Name, username, hash)
	if err != nil {
		return fmt.Errorf(`failed to update account %q@%q with rehashed password in UpdateAccountPassword: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}

	return nil
}

// VerifyAccountPassword verifies a password for an account in the room.
// If the account does not exist, returns ErrNoSuchAccount.
// Returns true if the password matches, false otherwise.
func (r *Room) VerifyAccountPassword(ctx context.Context, username common.NormalizedUsername, password string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isClosed {
		return false, ErrRoomClosed
	}

	record, has, err := r.storage.GetAccountByRoomAndUsername(ctx, r.Name, username)
	if err != nil {
		return false, fmt.Errorf(`failed to check if account %q@%q exists in VerifyAccountPassword: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}
	if !has {
		return false, ErrNoSuchAccount
	}

	matches, needsRehash, err := mcfpassword.VerifyPassword(password, record.PasswordHash)
	if err != nil {
		return false, fmt.Errorf(`failed to verify password for account %q@%q in VerifyAccountPassword: %w`,
			username.String(),
			r.Name.String(),
			err,
		)
	}

	// Rehash if needed.
	if needsRehash {
		var hash string
		hash, err = mcfpassword.HashPassword(password)
		if err != nil {
			return false, fmt.Errorf(`failed to rehash password for account %q@%q in VerifyAccountPassword: %w`,
				username.String(),
				r.Name.String(),
				err,
			)
		}

		err = r.storage.UpdateAccountPasswordHash(ctx, r.Name, username, hash)
		if err != nil {
			return false, fmt.Errorf(`failed to update rehashed password for account %q@%q in VerifyAccountPassword: %w`,
				username.String(),
				r.Name.String(),
				err,
			)
		}
	}

	return matches, nil
}

// handleConnect performs logic that needs to be done after a client connects.
// It returns quickly and does not lock on its own.
// The caller must lock before calling it.
func (r *Room) handleConnect(client *Client) {
	r.clients[client.Username.String()] = client

	r.Broadcast(pb.MsgType_MSG_TYPE_CLIENT_ONLINE, &pb.MsgClientOnline{
		Info: &pb.OnlineUserInfo{
			Username: client.Username.String(),
		},
	})

	r.logger.Info("client connected",
		"service", "room.Room",
		"room", r.Name.String(),
		"username", client.Username.String(),
	)
}

// handleDisconnect performs logic that needs to be done after a client disconnects.
// It returns quickly and does not lock on its own.
// The caller must lock before calling it.
// Duplicate calls for the same Client are no-op.
func (r *Room) handleDisconnect(client *Client) {
	unStr := client.Username.String()

	oldClient, has := r.clients[unStr]
	if !has || oldClient != client {
		return
	}

	delete(r.clients, unStr)

	// In case the connection was not closed, mark it as closed here.
	_ = client.conn.CloseWithReason("disconnected")

	r.Broadcast(pb.MsgType_MSG_TYPE_CLIENT_OFFLINE, &pb.MsgClientOffline{
		Username: client.Username.String(),
	})

	r.logger.Info("client disconnected",
		"service", "room.Room",
		"room", r.Name.String(),
		"username", client.Username.String(),
	)
}

// KickClientByUsername disconnects the client with the specified username.
// If there is no client with that username, this is a no-op.
func (r *Room) KickClientByUsername(username common.NormalizedUsername) error {
	r.mu.Lock()
	if r.isClosed {
		r.mu.Unlock()
		return ErrRoomClosed
	}

	client, has := r.clients[username.String()]
	if !has {
		r.mu.Unlock()
		return nil
	}

	r.handleDisconnect(client)
	r.mu.Unlock()

	if client != nil {
		return client.conn.CloseWithReason("kicked")
	}

	return nil
}
