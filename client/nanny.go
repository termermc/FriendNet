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

	// openCh is closed when the connection is currently open.
	// It is replaced with a new channel each time we transition away from open.
	openCh chan struct{}

	mu       sync.RWMutex
	isClosed bool

	certStore cert.Store
	address   string
	creds     room.Credentials

	isDaemonRunning bool
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
	n := &ConnNanny{
		logger: logger,

		certStore: certStore,
		address:   address,
		creds:     creds,

		openCh: make(chan struct{}),

		isDaemonRunning: false,
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
		if n.isClosed {
			n.mu.RUnlock()
			return nil, ErrConnNannyClosed
		}

		// Fast path if already open and non-nil.
		if n.state == ConnStateOpen && n.connOrNil != nil {
			c := n.connOrNil
			n.mu.RUnlock()
			return c, nil
		}

		ch := n.openCh
		n.mu.RUnlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
			// openCh closed; connection transitioned to open.
			// Loop to re-check state and grab the conn pointer.
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
	defer func() {
		if err := recover(); err != nil {
			n.logger.Error("panic in ConnNanny daemon",
				"address", n.address,
				"room", n.creds.Room,
				"username", n.creds.Username.String(),
				"err", err,
			)

			// Mark daemon as stopped, then attempt to restart.
			// Note: we restart in a new goroutine to avoid reusing the current stack.
			n.mu.Lock()
			orphanedConn := n.connOrNil
			n.isDaemonRunning = false
			n.connOrNil = nil
			n.state = ConnStateClosed
			// Ensure future WaitOpen calls block until we transition to open again.
			n.openCh = make(chan struct{})
			shouldRestart := !n.isClosed && n.shouldReconnect
			n.mu.Unlock()

			// If there was still a live connection during the crash, we need to close it.
			// Leaving it open would leave it orphaned, and weird things might happen.
			if orphanedConn != nil {
				_ = orphanedConn.Close()
			}

			if shouldRestart {
				n.startDaemonLockedSafe()
			}
		}
	}()

	for {
		n.mu.Lock()
		if n.isClosed || !n.shouldReconnect {
			n.isDaemonRunning = false
			n.mu.Unlock()
			return
		}

		n.state = ConnStateOpening
		n.mu.Unlock()

		conn, err := room.NewRoomConn(
			n.logger,
			room.MessageHandlersImpl,
			n.certStore,
			n.address,
			n.creds,
		)
		if err == nil {
			n.mu.Lock()
			n.state = ConnStateOpen
			n.connOrNil = conn
			close(n.openCh)
			n.mu.Unlock()

			// Wait for connection to close.
			<-conn.Context.Done()
		} else {
			n.logger.Error("failed to create room connection",
				"address", n.address,
				"room", n.creds.Room,
				"username", n.creds.Username.String(),
				"err", err,
			)

			n.mu.Lock()
			n.state = ConnStateClosed
			close(n.openCh)
			n.mu.Unlock()

			// TODO Apply backoff
		}

		// Clear conn value, reconnect if possible.
		n.mu.Lock()

		// Notify waiters that connection state has changed.
		n.openCh = make(chan struct{})

		n.connOrNil = nil
		n.state = ConnStateClosed
		if !n.shouldReconnect {
			n.isDaemonRunning = false
			n.mu.Unlock()
			return
		}
		n.mu.Unlock()
	}
}

// startDaemonLockedSafe starts the daemon if it isn't running.
// Caller must NOT hold n.mu when calling this helper; it acquires the lock itself.
func (n *ConnNanny) startDaemonLockedSafe() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.isClosed {
		return
	}
	if n.isDaemonRunning {
		return
	}
	n.isDaemonRunning = true
	go n.daemon()
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

	oldConn := n.connOrNil

	n.state = ConnStateClosed
	n.shouldReconnect = false

	// If someone is waiting in WaitOpen, unblock them by closing openCh (if not already closed)
	// so that they can observe closure.
	//
	// WaitOpen always checks isClosed under lock first, so it will return ErrConnNannyClosed.
	select {
	case <-n.openCh:
	default:
		close(n.openCh)
	}

	n.mu.Unlock()

	// Close old connection if present.
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
	defer n.mu.Unlock()
	if n.isClosed {
		return
	}

	n.shouldReconnect = true

	// Start daemon if needed.
	if !n.isDaemonRunning {
		n.isDaemonRunning = true
		go n.daemon()
	}
}

// Disconnect closes the current underlying connection and disabled reconnection.
// The underlying connection will not be reopened until Connect is called.
// No-op if the ConnNanny is closed.
func (n *ConnNanny) Disconnect() {
	n.mu.Lock()
	if n.isClosed {
		n.mu.Unlock()
		return
	}

	oldConn := n.connOrNil

	n.shouldReconnect = false
	n.state = ConnStateClosed
	n.connOrNil = nil

	n.mu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}
}
