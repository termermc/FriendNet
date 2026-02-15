package client

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"friendnet.org/client/cert"
	"friendnet.org/client/room"
)

var ErrConnNannyClosed = errors.New("conn nanny closed")
var ErrConnNotOpen = errors.New("connection not open")

// ConnState is the state of a connection.
type ConnState string

const (
	ConnStateClosed  ConnState = "closed"
	ConnStateOpening ConnState = "opening"
	ConnStateOpen    ConnState = "open"
)

// ConnNanny watches over a connection and manages reconnections, reporting state, etc.
type ConnNanny struct {
	logger *slog.Logger

	ctx       context.Context
	ctxCancel context.CancelFunc

	// openCh is closed when the connection is currently open.
	// It is replaced with a new channel each time we transition away from open.
	openCh chan struct{}

	mu       sync.RWMutex
	isClosed bool

	certStore cert.Store
	address   string
	creds     room.Credentials

	shouldReconnect bool
	connOrNil       *room.Conn

	state ConnState
}

// NewConnNanny creates a new ConnNanny with the specified server address and credentials.
// It automatically starts trying to connect after instantiation.
func NewConnNanny(
	logger *slog.Logger,
	certStore cert.Store,
	address string,
	creds room.Credentials,
) *ConnNanny {
	ctx, ctxCancel := context.WithCancel(context.Background())

	n := &ConnNanny{
		logger: logger,

		ctx:       ctx,
		ctxCancel: ctxCancel,

		certStore: certStore,
		address:   address,
		creds:     creds,

		openCh: make(chan struct{}),

		shouldReconnect: true,
		state:           ConnStateClosed,
	}

	go n.daemon()

	return n
}

// WaitOpen blocks until the underlying connection is open, ctx is done, or the nanny is closed.
// The returned *room.Conn is a snapshot; it may become unusable at any time due to disconnects.
// Callers should not retain it beyond a short-lived operation.
func (n *ConnNanny) WaitOpen(ctx context.Context) (*room.Conn, error) {
	for {
		n.mu.RLock()
		if !n.isClosed && n.state == ConnStateOpen && n.connOrNil != nil {
			c := n.connOrNil
			n.mu.RUnlock()
			return c, nil
		}

		// Conn was not open yet, snapshot what we need to wait for it.
		openCh := n.openCh
		isClosed := n.isClosed
		n.mu.RUnlock()

		if isClosed {
			return nil, ErrConnNannyClosed
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-n.ctx.Done():
			return nil, ErrConnNannyClosed
		case <-openCh:
			// openCh closed => transitioned to open; loop to grab conn snapshot.
		}
	}
}

// TryDo calls fn with the current connection snapshot if open; otherwise returns ErrConnNotOpen.
func (n *ConnNanny) TryDo(fn func(*room.Conn) error) error {
	n.mu.RLock()
	if n.isClosed {
		n.mu.RUnlock()
		return ErrConnNannyClosed
	}
	if n.state != ConnStateOpen || n.connOrNil == nil {
		n.mu.RUnlock()
		return ErrConnNotOpen
	}
	c := n.connOrNil
	n.mu.RUnlock()

	return fn(c)
}

// Do waits until the connection is open (or ctx done), then calls fn with the current connection snapshot.
// fn is called without holding the nanny lock.
func (n *ConnNanny) Do(
	ctx context.Context,
	fn func(ctx context.Context, c *room.Conn) error,
) error {
	c, err := n.WaitOpen(ctx)
	if err != nil {
		return err
	}
	return fn(ctx, c)
}

func (n *ConnNanny) daemon() {
	// Panic recovery: tear down state, close the orphaned conn if any, and restart if appropriate.
	defer func() {
		if rec := recover(); rec != nil {
			n.logger.Error("panic in ConnNanny daemon",
				"address", n.address,
				"room", n.creds.Room,
				"username", n.creds.Username.String(),
				"err", rec,
			)

			n.mu.Lock()
			orphanedConn := n.connOrNil
			n.connOrNil = nil
			n.state = ConnStateClosed
			n.openCh = make(chan struct{})
			shouldRestart := !n.isClosed && n.shouldReconnect
			n.mu.Unlock()

			if orphanedConn != nil {
				_ = orphanedConn.Close()
			}

			if shouldRestart {
				go n.daemon()
			}
		}
	}()

	for {
		n.mu.Lock()
		if n.isClosed {
			n.mu.Unlock()
			return
		}
		if !n.shouldReconnect {
			n.mu.Unlock()
			// Return, not doing anything until either Close() or Connect() flips shouldReconnect.
			// We don't have a dedicated "reconnect signal" channel yet; simplest
			// is to just return and let Connect() start a new daemon if desired.
			return
		}
		n.state = ConnStateOpening
		n.mu.Unlock()

		// Connect outside lock; may block.
		conn, err := room.NewRoomConn(
			n.logger,
			room.MessageHandlersImpl,
			n.certStore,
			n.address,
			n.creds,
		)
		if err != nil {
			n.logger.Error("failed to create room connection",
				"address", n.address,
				"room", n.creds.Room,
				"username", n.creds.Username.String(),
				"err", err,
			)

			n.mu.Lock()
			// Connection never opened, so we do not to close or recreate openCh.
			n.state = ConnStateClosed
			n.mu.Unlock()

			// TODO backoff
			continue
		}

		// Check if a Close or Disconnect happened since the connection opened.
		n.mu.Lock()
		if n.isClosed || !n.shouldReconnect {
			n.mu.Unlock()
			_ = conn.Close()
			// Loop will return next iteration if this condition stays true.
			continue
		}

		// Connection is open!
		// Set connection and state, then signal to waiters that it is open.
		n.connOrNil = conn
		n.state = ConnStateOpen
		select {
		case <-n.openCh:
		default:
			close(n.openCh)
		}
		n.mu.Unlock()

		// Wait for connection to end.
		<-conn.Context.Done()

		// Transition away from open: clear conn and reset openCh so WaitOpen blocks again.
		n.mu.Lock()
		if n.connOrNil == conn {
			n.connOrNil = nil
		}
		n.state = ConnStateClosed
		n.openCh = make(chan struct{})
		n.mu.Unlock()

		// Loop will reconnect if shouldReconnect remains true.
	}
}

// Close closes the ConnNanny, and as a result, the underlying connection.
// If you want to disconnect the underlying connection, use Disconnect.
// Subsequent calls are no-op.
func (n *ConnNanny) Close() error {
	n.mu.Lock()
	if n.isClosed {
		n.mu.Unlock()
		return nil
	}
	n.isClosed = true
	n.shouldReconnect = false
	n.state = ConnStateClosed

	oldConn := n.connOrNil
	n.connOrNil = nil

	n.ctxCancel()

	n.mu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}

	return nil
}

// IsClosed returns whether the ConnNanny itself is closed.
// To check whether the underlying connection is closed, use State.
func (n *ConnNanny) IsClosed() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isClosed
}

// State returns the underlying connection state.
func (n *ConnNanny) State() ConnState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.isClosed {
		return ConnStateClosed
	}
	return n.state
}

// Connect schedules a reconnection (if not already connected), and enables automatic reconnection.
// No-op if the ConnNanny is closed.
func (n *ConnNanny) Connect() {
	n.mu.Lock()
	if n.isClosed {
		n.mu.Unlock()
		return
	}
	was := n.shouldReconnect
	n.shouldReconnect = true
	n.mu.Unlock()

	// If we were previously disconnected (daemon returned), start it again.
	if !was {
		go n.daemon()
	}
}

// Disconnect closes the current underlying connection and disables reconnection.
// The underlying connection will not be reopened until Connect is called.
// No-op if the ConnNanny is closed.
func (n *ConnNanny) Disconnect() {
	n.mu.Lock()
	if n.isClosed {
		n.mu.Unlock()
		return
	}

	oldConn := n.connOrNil
	n.connOrNil = nil

	n.shouldReconnect = false
	n.state = ConnStateClosed

	// Ensure WaitOpen blocks until we open again.
	n.openCh = make(chan struct{})

	n.mu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}
}
