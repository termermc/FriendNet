package tcpstyle

import (
	"context"
	"errors"
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
type AcceptFunc func() (net.Conn, error)

// ConnManager manages connections and streams for TCP-style plaintext connections.
// It is responsible for both listening and dialing.
type ConnManager struct {
	mu       sync.RWMutex
	isClosed bool

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
func NewConnManager(dial DialFunc, accept AcceptFunc) *ConnManager {
	return &ConnManager{
		dial:     dial,
		accept:   accept,
		conns:    make(map[int64]*Conn),
		acceptCh: make(chan *Conn),
	}
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
