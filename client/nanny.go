package client

import (
	"log/slog"
	"sync"

	"friendnet.org/client/room"
)

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

	mu       sync.RWMutex
	isClosed bool

	address string
	creds   room.Credentials

	shouldReconnect bool
	connOrNil       *room.Conn

	state ConnState
}

// NewConnNanny creates a new ConnNanny with the specified server address and credentials.
// It automatically starts trying to connect after instantiation.
func NewConnNanny(logger *slog.Logger, address string, creds room.Credentials) *ConnNanny {
	n := &ConnNanny{
		logger: logger,

		address: address,
		creds:   creds,

		shouldReconnect: true,
		state:           ConnStateClosed,
	}

	go n.daemon()

	return n
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

			// Attempt to relaunch daemon.
			// This will eventually cause a stack overflow if it happens too many times,
			// but it's easy enough to do this to recover from panics without breaking
			// everything.
			n.mu.Lock()
			n.connOrNil = nil
			n.mu.Unlock()
			go n.daemon()
		}
	}()

	for {
		n.mu.Lock()
		if n.isClosed || n.connOrNil != nil {
			// Closed or another daemon is running.
			n.mu.Unlock()
			break
		}

		n.state = ConnStateOpening
		// TODO Create and put conn
		n.state = ConnStateOpen

		n.mu.Unlock()

		// TODO Listen.
		// Only return after close or terminal error.
		// TODO If there was an error, apply some kind of backoff timeout for connecting.

		// Clear conn value, reconnect if possible.
		n.mu.Lock()
		n.connOrNil = nil
		n.state = ConnStateClosed
		if !n.shouldReconnect {
			n.mu.Unlock()
			break
		}
		n.mu.Unlock()
	}
}

// Close closes the ConnNanny, and as a result, the underlying connection.
// If you want to disconnect the underlying connection, use Disconnect.
// Subsequent calls are no-op.
func (n *ConnNanny) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.isClosed {
		return nil
	}
	n.isClosed = true

	n.state = ConnStateClosed
	n.shouldReconnect = false

	// Close connection if present.
	if n.connOrNil != nil {
		_ = n.connOrNil.Close()
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

	// If there is already a connection, the daemon will exit itself.
	go n.daemon()
}

// Disconnect closes the current underlying connection and disabled reconnection.
// The underlying connection will not be reopened until Connect is called.
// No-op if the ConnNanny is closed.
func (n *ConnNanny) Disconnect() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.isClosed {
		return
	}

	n.shouldReconnect = false
	n.state = ConnStateClosed

	if n.connOrNil != nil {
		_ = n.connOrNil.Close()
	}
}
