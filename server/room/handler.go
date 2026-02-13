package room

import (
	"context"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// PingHandler handles an incoming ping request.
// Implementations must write a MSG_TYPE_PONG before returning.
type PingHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error

// OpenOutboundProxyHandler handles an open outbound proxy request.
// Implementations must follow the documentation on MSG_TYPE_OPEN_OUTBOUND_PROXY.
type OpenOutboundProxyHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgOpenOutboundProxy]) error

// GetOnlineUsersHandler handles an incoming get online users request.
// Implementations must write one or more MSG_TYPE_ONLINE_USERS messages before returning.
type GetOnlineUsersHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetOnlineUsers]) error

// ClientMessageHandlers handlers for incoming client messages.
// Handlers are called after receiving a new incoming bidi from a client with the corresponding message type.
//
// Important: Handlers must assume that the underlying bidi will be closed after the handler returns.
// References to the bidi or client must not be held after the handler returns.
type ClientMessageHandlers struct {
	OnPing              PingHandler
	OnOpenOutboundProxy OpenOutboundProxyHandler
	OnGetOnlineUsers    GetOnlineUsersHandler
}
