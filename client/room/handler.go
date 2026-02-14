package room

import (
	"context"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// PingHandler handles an incoming ping request.
// Implementations must write a MSG_TYPE_PONG before returning.
type PingHandler func(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error

// ServerToClientMessageHandlers is handlers for incoming S2C messages.
// Handlers are called after receiving a new incoming bidi from a client with the corresponding message type.
//
// Important: Handlers must assume that the underlying bidi will be closed after the handler returns.
// References to the bidi or client must not be held after the handler returns.
type ServerToClientMessageHandlers struct {
	OnPing PingHandler
}

// ClientToClientMessageHandlers is handlers for incoming C22 messages.
// The messages may have come from a direct connection or from an inbound proxy stream.
// Handlers are called after receiving a new incoming bidi from a client with the corresponding message type.
//
// Important: Handlers must assume that the underlying bidi will be closed after the handler returns.
// References to the bidi or client must not be held after the handler returns.
type ClientToClientMessageHandlers struct {
	// TODO C2C handlers
}

// MessageHandlers contains all message handlers for a room connection.
type MessageHandlers struct {
	ServerToClientMessageHandlers
	ClientToClientMessageHandlers
}
