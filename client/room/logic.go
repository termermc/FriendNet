package room

import (
	"context"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// MessageHandlersImpl implements all handlers in MessageHandlers.
var MessageHandlersImpl = MessageHandlers{
	S2cOnPing: func(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error {
		return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{
			SentTs: time.Now().UnixMilli(),
		})
	},
}
