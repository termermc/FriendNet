package room

import (
	"context"
	"io"

	"friendnet.org/client/share"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// Logic exposes handlers for incoming client messages, both S2C and C2C.
//
// Each handler is provided with the information it needs to return a response.
// Handlers must not hold references to the bidi or connection outside the handler.
// Handlers do not need to close bidis; they are closed by the caller after the handler returns.
type Logic interface {
	io.Closer

	// OnPing handles an incoming ping request.
	//
	// S2C, C2C
	OnPing(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error
}

// LogicImpl implements Logic.
type LogicImpl struct {
	shares *share.ServerShareManager
}

var _ Logic = (*LogicImpl)(nil)

func NewLogicImpl(shares *share.ServerShareManager) *LogicImpl {
	return &LogicImpl{
		shares: shares,
	}
}

func (l *LogicImpl) Close() error {
	return l.shares.Close()
}

func (l *LogicImpl) OnPing(_ context.Context, _ *Conn, bidi protocol.ProtoBidi, _ *protocol.TypedProtoMsg[*pb.MsgPing]) error {
	return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{})
}
