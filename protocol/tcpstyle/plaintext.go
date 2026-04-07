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
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

// protoVersion is the version of the protocol used by this implementation.
// The first int64 of the open message (the connection ID field) is the protocol version.
const protoVersion int64 = 1

var emptyAppErr = &quic.ApplicationError{
	Remote:       true,
	ErrorMessage: "",
	ErrorCode:    0,
}

// ErrConnManagerClosed is returned by ConnManager methods when the ConnManager is closed.
var ErrConnManagerClosed = errors.New("ConnManager is closed")

// DialFunc is a function that dials an address and returns an outgoing TCP-style plaintext connection.
// If the underlying connection is closed, it must return net.ErrClosed.
type DialFunc func(addr string) (net.Conn, error)

// AcceptFunc is a function that returns an incoming TCP-style plaintext connection.
// If there are no more connections to accept, it must return net.ErrClosed.
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
	closer io.Closer,
	dial DialFunc,
	accept AcceptFunc,
	connInactivityTimeout time.Duration,
) *ConnManager {
	m := &ConnManager{
		logger: logger,

		closer: closer,
		dial:   dial,
		accept: accept,

		conns: make(map[int64]*Conn),

		acceptCh: make(chan *Conn),
	}

	go func() {
		for !m.isClosed {
			rawConn, err := m.accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					_ = m.Close()
					return
				}

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
					// The connection ID is used as the protocol version field for this op.
					if header.ConnId != protoVersion {
						// Plaintext protocol version mismatch.
						_ = rawConn.Close()
						return
					}

					// Generate a connection ID.
					var idBytes [8]byte
					_, _ = rand.Read(idBytes[:])
					id := int64(binary.LittleEndian.Uint64(idBytes[:]))

					m.mu.RLock()
					if m.isClosed {
						m.mu.RUnlock()
						_ = rawConn.Close()
						return
					}
					m.mu.RUnlock()

					// Wait for the connection to be accepted.
					c := newConn(m, id, rawConn.RemoteAddr())
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

	// Periodically check for connections that have been inactive for too long and close them.
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			if m.isClosed {
				return
			}

			m.mu.RLock()
			for _, c := range m.conns {
				if time.Since(c.lastActivity) > connInactivityTimeout {
					go func() {
						_ = c.CloseWithReason("connection inactivity timeout")
					}()
				}
			}
			m.mu.RUnlock()
		}
	}()

	return m
}

// Accept waits for a new incoming connection and returns it.
// Returns ErrConnManagerClosed if the ConnManager is closed.
func (m *ConnManager) Accept(ctx context.Context) (*Conn, error) {
	m.mu.RLock()
	if m.isClosed {
		m.mu.RUnlock()
		return nil, ErrConnManagerClosed
	}
	m.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c := <-m.acceptCh:
		if c == nil {
			return nil, ErrConnManagerClosed
		}

		return c, nil
	}
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

	close(m.acceptCh)

	return m.closer.Close()
}

// Dial makes a new outgoing connection to the specified address.
func (m *ConnManager) Dial(addr string) (*Conn, error) {
	m.mu.RLock()
	if m.isClosed {
		m.mu.RUnlock()
		return nil, ErrConnManagerClosed
	}
	m.mu.RUnlock()

	openConn, err := m.dial(addr)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			_ = m.Close()
			return nil, ErrConnManagerClosed
		}

		return nil, err
	}
	defer func() {
		_ = openConn.Close()
	}()

	err = writeStreamHeader(openConn, streamHeader{
		ConnId: protoVersion,
		Op:     streamOpOpenConn,
	})
	if err != nil {
		return nil, err
	}

	// Read connection ID.
	var idBytes [8]byte
	_, err = io.ReadFull(openConn, idBytes[:])
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Connection was closed on the other side.
			return nil, emptyAppErr
		}

		return nil, err
	}
	id := int64(binary.LittleEndian.Uint64(idBytes[:]))

	conn := newConn(m, id, openConn.RemoteAddr())

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

func (m *ConnManager) removeConn(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, id)
}

// Conn is an implementation of protocol.ProtoConn that uses TCP-style plaintext connections under the hood to emulate
// QUIC bidi streams.
type Conn struct {
	mu       sync.Mutex
	isClosed bool

	lastActivity time.Time

	ctx       context.Context
	ctxCancel context.CancelFunc

	m    *ConnManager
	id   int64
	addr net.Addr

	incomingBidiCh chan net.Conn
}

var _ protocol.ProtoConn = (*Conn)(nil)

func newConn(m *ConnManager, id int64, addr net.Addr) *Conn {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &Conn{
		lastActivity: time.Now(),

		ctx:       ctx,
		ctxCancel: ctxCancel,

		m:    m,
		id:   id,
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

func (c *Conn) CloseWithReason(string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return nil
	}

	// Remove self from manager conn map.
	c.m.removeConn(c.id)

	c.isClosed = true
	c.ctxCancel()

	go func() {
		conn, err := c.m.dial(c.addr.String())
		if err != nil {
			// It is what it is.
			return
		}

		_ = writeStreamHeader(conn, streamHeader{
			ConnId: c.id,
			Op:     streamOpCloseConn,
		})
		_ = conn.Close()
	}()

	return nil
}

func (c *Conn) OpenBidiWithMsg(typ pb.MsgType, msg proto.Message) (bidi protocol.ProtoBidi, err error) {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return protocol.ProtoBidi{}, emptyAppErr
	}
	c.mu.Unlock()

	netConn, err := c.m.dial(c.addr.String())
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			_ = c.CloseWithReason("underlying dialer closed")
			return bidi, emptyAppErr
		}

		return bidi, err
	}

	err = writeStreamHeader(netConn, streamHeader{
		ConnId: c.id,
		Op:     streamOpOpenBidi,
	})
	if err != nil {
		_ = netConn.Close()
		return bidi, err
	}

	s := newStream(c.ctx, c, netConn)
	bidi = protocol.ProtoBidi{
		Stream:            s,
		ProtoStreamReader: protocol.NewProtoStreamReader(s),
		ProtoStreamWriter: protocol.NewProtoStreamWriter(s),
	}

	err = bidi.Write(typ, msg)
	if err != nil {
		_ = bidi.Close()
		return bidi, err
	}

	return bidi, nil
}

func (c *Conn) WaitForBidi(ctx context.Context) (protocol.ProtoBidi, error) {
	select {
	case <-ctx.Done():
		return protocol.ProtoBidi{}, emptyAppErr
	case netConn := <-c.incomingBidiCh:
		c.mu.Lock()
		c.lastActivity = time.Now()
		c.mu.Unlock()

		s := newStream(ctx, c, netConn)
		return protocol.ProtoBidi{
			Stream:            s,
			ProtoStreamReader: protocol.NewProtoStreamReader(s),
			ProtoStreamWriter: protocol.NewProtoStreamWriter(s),
		}, nil
	}
}

func (c *Conn) SendAndReceive(typ pb.MsgType, msg proto.Message) (*protocol.UntypedProtoMsg, error) {
	bidi, err := c.OpenBidiWithMsg(typ, msg)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	return bidi.Read()
}

func (c *Conn) SendAndReceiveAck(typ pb.MsgType, msg proto.Message) error {
	reply, err := c.SendAndReceive(typ, msg)
	if err != nil {
		return err
	}

	if reply.Type != pb.MsgType_MSG_TYPE_ACKNOWLEDGED {
		return protocol.UnexpectedMsgTypeError{
			Expected: pb.MsgType_MSG_TYPE_ACKNOWLEDGED,
			Actual:   reply.Type,
		}
	}

	return nil
}

// stream implements protocol.ProtoBidiStream.
type stream struct {
	c         *Conn
	netConn   net.Conn
	ctx       context.Context
	ctxCancel context.CancelFunc
}

var _ protocol.ProtoBidiStream = (*stream)(nil)

func newStream(ctx context.Context, c *Conn, netConn net.Conn) *stream {
	ctx, ctxCancel := context.WithCancel(ctx)

	return &stream{
		c:         c,
		netConn:   netConn,
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}
}

func (s *stream) Close() error {
	s.ctxCancel()
	_ = s.netConn.Close()

	return nil
}

func (s *stream) Read(p []byte) (n int, err error) {
	select {
	case <-s.ctx.Done():
		return 0, emptyAppErr
	default:
		n, err = s.netConn.Read(p)
		if err != nil {
			if n == 0 {
				return 0, emptyAppErr
			}

			return n, err
		}

		s.c.m.mu.Lock()
		defer s.c.m.mu.Unlock()
		s.c.lastActivity = time.Now()

		return n, nil
	}
}

func (s *stream) Write(p []byte) (n int, err error) {
	select {
	case <-s.ctx.Done():
		return 0, emptyAppErr
	default:
		n, err = s.netConn.Write(p)
		if err != nil {
			if n == 0 {
				return 0, emptyAppErr
			}

			return n, err
		}

		s.c.m.mu.Lock()
		defer s.c.m.mu.Unlock()
		s.c.lastActivity = time.Now()

		return n, nil
	}
}

func (s *stream) CancelRead(quic.StreamErrorCode) {
	// Error code part is not implemented.
	_ = s.Close()
}
