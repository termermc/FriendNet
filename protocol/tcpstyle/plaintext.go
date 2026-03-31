package tcpstyle

import (
	"context"
	"net"
	"sync"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"google.golang.org/protobuf/proto"
)

type ConnManager struct {
}

// AcceptFunc is a function that returns a TCP-style plaintext connection.
// It used by PlaintextConn to accept connections used for incoming bidi streams,
// and by Listener to accept incoming connections.
type AcceptFunc func() (net.Conn, error)

type Listener struct {
	mu sync.RWMutex

	acceptFn AcceptFunc

	conns map[int64]*Conn
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

// ConnFunc is a function that returns a TCP-style plaintext connection.
// It used by PlaintextConn to create connections used for outgoing bidi streams.
type ConnFunc func() (net.Conn, error)

type Conn struct {
	addr net.Addr

	// The connection ID.
	// If someone were to guess this, it would be able to masquerade as the same connection.
	// Here's the time it would take to guess the connection ID if you could verify it once every
	connId int64

	connFn ConnFunc
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
