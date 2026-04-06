package tcpstyle

import (
	"context"
	"crypto/rand"
	"encoding/binary"
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

// Do not remove or rearrange streamOp values, or the protocol will break!
const (
	streamOpOpenConn streamOp = iota
	streamOpCloseConn
	streamOpOpenBidi
)

type streamHeader struct {
	ConnId int64
	Op     streamOp
}

func readStreamHeader(r io.Reader) (streamHeader, error) {
	var header streamHeader
	err := binary.Read(r, binary.LittleEndian, &header)
	if err != nil {
		return streamHeader{}, err
	}
	return header, nil
}
func writeStreamHeader(w io.Writer, header streamHeader) error {
	return binary.Write(w, binary.LittleEndian, header)
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
			rawConn, err := m.accept()
			if err != nil {
				m.logger.Error("failed to accept connection",
					"service", "tcpstyle.ConnManager",
					"err", err,
				)
				continue
			}
			if rawConn == nil {
				_ = m.Close()
				return
			}
			if m.isClosed {
				return
			}

			// Handle connection.
			go func() {
				header, err := readStreamHeader(rawConn)
				if err != nil {
					_ = rawConn.Close()
					return
				}

				switch header.Op {
				case streamOpOpenConn:
					// Connection ID should be 0 here.
					if header.ConnId != 0 {
						_ = rawConn.Close()
						return
					}

					// Generate a connection ID.
					var idBytes [8]byte
					_, _ = rand.Read(idBytes[:])
					id := int64(binary.LittleEndian.Uint64(idBytes[:]))

					// Wait for the connection to be accepted.
					c := newConn(m, rawConn.RemoteAddr())
					m.acceptCh <- c

					// Connection accepted, add to map and write success.
					m.mu.Lock()
					m.conns[id] = c
					m.mu.Unlock()

					_, _ = rawConn.Write(idBytes[:])
					_ = rawConn.Close()
				case streamOpCloseConn:
					m.mu.RLock()
					c, has := m.conns[header.ConnId]
					m.mu.RUnlock()
					if !has {
						_ = rawConn.Close()
						return
					}
					_ = c.CloseWithReason("closed by peer")
				case streamOpOpenBidi:
					m.mu.RLock()
					c, has := m.conns[header.ConnId]
					m.mu.RUnlock()
					if !has {
						_ = rawConn.Close()
						return
					}

					// Pass raw connection to conn to be used as a bidi stream.
					c.sendBidi(rawConn)
				default:
					// Unknown op.
					_ = rawConn.Close()
				}
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
func (m *ConnManager) Dial(addr string) (*Conn, error) {
	openConn, err := m.dial(addr)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = openConn.Close()
	}()

	err = writeStreamHeader(openConn, streamHeader{
		ConnId: 0,
		Op:     streamOpOpenConn,
	})
	if err != nil {
		return nil, err
	}

	// Read connection ID.
	var idBytes [8]byte
	_, err = io.ReadFull(openConn, idBytes[:])
	if err != nil {
		return nil, err
	}
	id := int64(binary.LittleEndian.Uint64(idBytes[:]))

	conn := newConn(m, openConn.RemoteAddr())

	m.mu.Lock()
	defer m.mu.Unlock()

	_, has := m.conns[id]
	if has {
		// The server sent an ID we already have.
		// In the real world, this should never happen, so the server implementation must be doing something weird.
		// We'll just drop the connection and let it time out on the other end.
		return nil, errors.New("server sent duplicate connection ID")
	}

	// Connection succeeded; add to map and return.
	m.conns[id] = conn
	return conn, nil
}

// Conn is an implementation of protocol.ProtoConn that uses TCP-style plaintext connections under the hood to emulate
// QUIC bidi streams.
type Conn struct {
	mu sync.RWMutex

	m    *ConnManager
	addr net.Addr

	incomingBidiCh chan net.Conn
}

var _ protocol.ProtoConn = (*Conn)(nil)

func newConn(m *ConnManager, addr net.Addr) *Conn {
	return &Conn{
		m:    m,
		addr: addr,

		incomingBidiCh: make(chan net.Conn),
	}
}

func (c *Conn) sendBidi(conn net.Conn) {
	c.incomingBidiCh <- conn
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *Conn) CloseWithReason(s string) error {
	//TODO implement me
	panic("implement me")
}

func (c *Conn) OpenBidiWithMsg(typ pb.MsgType, msg proto.Message) (bidi protocol.ProtoBidi, err error) {
	//TODO implement me
	panic("implement me")
}

func (c *Conn) WaitForBidi(ctx context.Context) (protocol.ProtoBidi, error) {
	//TODO implement me
	panic("implement me")
}

func (c *Conn) SendAndReceive(typ pb.MsgType, msg proto.Message) (*protocol.UntypedProtoMsg, error) {
	//TODO implement me
	panic("implement me")
}

func (c *Conn) SendAndReceiveAck(typ pb.MsgType, msg proto.Message) error {
	//TODO implement me
	panic("implement me")
}
