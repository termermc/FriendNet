package room

import (
	"context"
	"time"

	"friendnet.org/common"
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

	OnGetOnlineUsers: func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetOnlineUsers]) error {
		const pageSize = 50

		// Snapshot clients and get their statuses.
		clients := client.Room.snapshotClients()
		statuses := make([]*pb.OnlineUserStatus, len(clients))
		for i, client := range clients {
			statuses[i] = &pb.OnlineUserStatus{
				Username: client.Username.String(),
			}
		}

		// Send pages of statuses.
		sent := 0
		for sent < len(clients) {
			end := sent + pageSize
			if end > len(clients) {
				end = len(clients)
			}

			err := bidi.Write(pb.MsgType_MSG_TYPE_ONLINE_USERS, &pb.MsgOnlineUsers{
				Users: statuses[sent:end],
			})
			if err != nil {
				return err
			}

			// We could have sent less than pageSize, but in that case it would break anyway, so we don't care about being accurate here.
			sent += pageSize
		}

		return nil
	},

	OnOpenOutboundProxy: func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgOpenOutboundProxy]) error {
		// Create proxy.
		const magicProxyTargetNotOnlineStatus = 101

		// Validate username.
		targetUsername, usernameValid := common.NormalizeUsername(msg.Payload.TargetUsername)
		if !usernameValid {
			bidi.Stream.CancelRead(magicProxyTargetNotOnlineStatus)
			return nil
		}

		proxy, err := NewClientProxy(
			client.Room,
			client.Username,
			targetUsername,
			bidi,
		)
		if err != nil {
			return err
		}

		return proxy.Run()
	},
}
