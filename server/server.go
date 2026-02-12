package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"friendnet.org/protocol"
	"friendnet.org/server/lobby"
	"friendnet.org/server/room"
	"friendnet.org/server/storage"
)

// Server is a FriendNet server.
//
// A FriendNet server contains rooms, each one with its own accounts and isolated environment.
// Before entering a room, each new connection is sent to the lobby, where version negotiation and
// authentication are performed. Once the connection is authenticated, it is sent to the
// appropriate room.
type Server struct {
	mu       sync.Mutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	logger  *slog.Logger
	storage *storage.Storage
	lobby   *lobby.Lobby

	// The server's room.Manager instance.
	// Do not update or close it.
	RoomManager *room.Manager
}

// NewServer creates a new FriendNet server.
// It uses the specified storage instance.
// It does not start listening until Listen is called.
// Note that Server.Close does not close the storage instance.
func NewServer(
	logger *slog.Logger,
	storage *storage.Storage,
) (*Server, error) {
	if storage == nil {
		panic("storage cannot be nil")
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	roomMgr, err := room.NewManager(ctx, logger, storage, room.ClientMessageHandlersImpl)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	l := lobby.NewLobby(
		logger,
		storage,
		roomMgr,
		lobby.DefaultTimeout,
		protocol.CurrentProtocolVersion,
	)

	s := &Server{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		logger:  logger,
		storage: storage,
		lobby:   l,

		RoomManager: roomMgr,
	}

	return s, nil
}

// Close closes the server.
// Subsequent calls are no-op.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isClosed {
		return nil
	}

	_ = s.RoomManager.Close()

	s.ctxCancel()

	return nil
}

// ListenWith starts listening with the specified listener.
// This function can be called concurrently with other listeners to listen on multiple interfaces.
// Returns nil when Server.Close is called.
//
// Does not close the listener.
//
// Use Listen instead if you want to use the default listener.
func (s *Server) ListenWith(listener protocol.ProtoListener) error {
	for {
		conn, err := listener.Accept(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		s.lobby.Onboard(conn)
	}
}

// Listen starts listening on the specified address.
// The address must be in HOST:PORT format, e.g. "127.0.0.1:20038".
// IPv6 addresses must be enclosed in square brackets, e.g. "[::1]:20038".
// This function can be called concurrently with other listeners to listen on multiple interfaces.
// Returns nil when Server.Close is called.
func (s *Server) Listen(address string, tlsCfg *tls.Config) error {
	listener, err := protocol.NewQuicProtoListener(address, tlsCfg)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	return s.ListenWith(listener)
}
