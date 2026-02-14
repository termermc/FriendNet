package room

import (
	"context"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// S2cPingHandler handles an incoming ping request.
// Implementations must write a MSG_TYPE_PONG before returning.
type S2cPingHandler func(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error

// MessageHandlers is handlers for incoming messages.
// Handlers are called after receiving a new incoming bidi from a client with the corresponding message type.
//
// Important: Handlers must assume that the underlying bidi will be closed after the handler returns.
// References to the bidi or client must not be held after the handler returns.
type MessageHandlers struct {
	S2cOnPing S2cPingHandler
}
