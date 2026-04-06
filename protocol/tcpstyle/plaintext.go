package tcpstyle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"google.golang.org/protobuf/proto"
)

// ErrConnManagerClosed is returned by ConnManager methods when the ConnManager is closed.
var ErrConnManagerClosed = errors.New("ConnManager is closed")

// DialFunc is a function that dials an address and returns an outgoing TCP-style plaintext connection.
type DialFunc func(addr string) (net.Conn, error)

// AcceptFunc is a function that returns an incoming TCP-style plaintext connection.
// If there are no more connections to accept, it must return (nil, nil).
type AcceptFunc func() (net.Conn, error)

type streamOp uint8

const (
	streamOpOpenConn streamOp = iota
	streamOpAcceptConn
	streamOpRejectConn
	streamCloseConn
	streamOpOpenBidi
)

type streamHeader struct {
	ConnId int64
	Op     streamOp
}

// ConnManager manages connections and streams for TCP-style plaintext connections.
// It is responsible for both listening and dialing.
type ConnManager struct {
	mu       sync.RWMutex
	isClosed bool

	logger *slog.Logger

	closer io.Closer
	dial   DialFunc
	accept AcceptFunc

	// Confirmed connections.
	// It doesn't matter whether they were outgoing or incoming originally.
	conns map[int64]*Conn

	// Listeners read from this channel, and incoming connections get sent to it.
	// An incoming connection is only confirmed after it is received by a listener.
	acceptCh chan *Conn
}

// NewConnManager creates a new ConnManager with the provided dial and accept functions.
func NewConnManager(
	logger *slog.Logger,
	close io.Closer,
	dial DialFunc,
	accept AcceptFunc,
) *ConnManager {
	m := &ConnManager{
		logger: logger,

		closer: close,
		dial:   dial,
		accept: accept,

		conns: make(map[int64]*Conn),

		acceptCh: make(chan *Conn),
	}

	go func() {
		for !m.isClosed {
			conn, err := m.accept()
			if err != nil {
				m.logger.Error("failed to accept connection",
					"service", "tcpstyle.ConnManager",
					"err", err,
				)
				continue
			}
			if conn == nil {
				_ = m.Close()
				return
			}
			if m.isClosed {
				return
			}

			// Handle connection.
			go func() {
				// TODO Read header, figure out if it's a new or existing connection.
				_ = conn
			}()
		}
	}()

	return m
}

// Close closes the ConnManager if it is not already closed.
// It calls Close on the closer passed to the ConnManager when it was created exactly once.
// If the ConnManager is already closed, it is a no-op.
func (m *ConnManager) Close() error {
	m.mu.Lock()
	if m.isClosed {
		m.mu.Unlock()
		return nil
	}
	m.isClosed = true
	m.mu.Unlock()

	return m.closer.Close()
}

// Dial makes a new outgoing connection to the specified address.
func (m *ConnManager) Dial(addr string) (net.Conn, error) {
	// TODO implement me
	panic("implement me")
}

type Conn struct {
	addr net.Addr

	mu sync.RWMutex
}

var _ protocol.ProtoConn = (*Conn)(nil)

func (t *Conn) RemoteAddr() net.Addr {
	return t.addr
}

func (t *Conn) CloseWithReason(s string) error {
	//TODO implement me
	panic("implement me")
}

func (t *Conn) OpenBidiWithMsg(typ pb.MsgType, msg proto.Message) (bidi protocol.ProtoBidi, err error) {
	//TODO implement me
	panic("implement me")
}

func (t *Conn) WaitForBidi(ctx context.Context) (protocol.ProtoBidi, error) {
	//TODO implement me
	panic("implement me")
}

func (t *Conn) SendAndReceive(typ pb.MsgType, msg proto.Message) (*protocol.UntypedProtoMsg, error) {
	//TODO implement me
	panic("implement me")
}

func (t *Conn) SendAndReceiveAck(typ pb.MsgType, msg proto.Message) error {
	//TODO implement me
	panic("implement me")
}
