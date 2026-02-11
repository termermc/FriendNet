package room

import (
	"context"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// ClientMessageHandlersImpl implements all handlers in ClientMessageHandlers.
var ClientMessageHandlersImpl = ClientMessageHandlers{
	OnPing: func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error {
		return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{
			SentTs: time.Now().Unix(),
		})
	},
}
