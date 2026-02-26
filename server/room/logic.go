package room

import (
	"context"
	"strings"
	"time"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// Logic exposes handlers for incoming C2S messages.
//
// Each handler is provided with the information it needs to return a response.
// Handlers must not hold references to the bidi or connection outside the handler.
// Handlers do not need to close bidis; they are closed by the caller after the handler returns.
type Logic interface {
	// OnPing handles an incoming ping request.
	// Implementations must follow the documentation on MSG_TYPE_PING.
	OnPing(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgPing],
	) error

	// OnOpenOutboundProxy handles an open outbound proxy request.
	// Implementations must follow the documentation on MSG_TYPE_OPEN_OUTBOUND_PROXY.
	OnOpenOutboundProxy(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgOpenOutboundProxy],
	) error

	// OnGetOnlineUsers handles an incoming get online users request.
	// Implementations must write one or more MSG_TYPE_ONLINE_USERS messages before returning.
	OnGetOnlineUsers(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgGetOnlineUsers],
	) error

	// OnGetPublicIp handles an incoming get public IP request.
	// Implementations must follow the documentation on MSG_TYPE_GET_PUBLIC_IP.
	OnGetPublicIp(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgGetPublicIp],
	) error

	// OnGetDirectConnHandshakeToken handles an incoming get direct connection handshake token request.
	// Implementations must follow the documentation on MSG_TYPE_GET_DIRECT_CONN_HANDSHAKE_TOKEN.
	OnGetDirectConnHandshakeToken(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgGetDirectConnHandshakeToken],
	) error
}

type LogicImpl struct {
}

var _ Logic = (*LogicImpl)(nil)

func NewLogicImpl() *LogicImpl {
	return &LogicImpl{}
}

func (l LogicImpl) OnPing(_ context.Context, _ *Client, bidi protocol.ProtoBidi, _ *protocol.TypedProtoMsg[*pb.MsgPing]) error {
	return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{
		SentTs: time.Now().Unix(),
	})
}

func (l LogicImpl) OnOpenOutboundProxy(_ context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgOpenOutboundProxy]) error {
	// Validate username.
	targetUsername, usernameValid := common.NormalizeUsername(msg.Payload.TargetUsername)
	if !usernameValid {
		bidi.Stream.CancelRead(protocol.ProxyPeerUnreachableStreamErrorCode)
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
	defer func() {
		_ = proxy.Close()
	}()

	return proxy.Run()
}

func (l LogicImpl) OnGetOnlineUsers(_ context.Context, client *Client, bidi protocol.ProtoBidi, _ *protocol.TypedProtoMsg[*pb.MsgGetOnlineUsers]) error {
	const pageSize = 50

	// Snapshot clients and get their statuses.
	clients := client.Room.GetAllClients()
	statuses := make([]*pb.OnlineUserInfo, len(clients))
	for i, c := range clients {
		statuses[i] = &pb.OnlineUserInfo{
			Username: c.Username.String(),
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
}

func (l LogicImpl) OnGetPublicIp(_ context.Context, client *Client, bidi protocol.ProtoBidi, _ *protocol.TypedProtoMsg[*pb.MsgGetPublicIp]) error {
	remote := client.RemoteAddr().String()

	var addr string
	if colonIdx := strings.IndexRune(remote, ':'); colonIdx != -1 {
		addr = remote[:colonIdx]
	} else {
		addr = remote
	}

	return bidi.Write(pb.MsgType_MSG_TYPE_PUBLIC_IP, &pb.MsgPublicIp{
		PublicIp: addr,
	})
}

func (l LogicImpl) OnGetDirectConnHandshakeToken(_ context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetDirectConnHandshakeToken]) error {
	target, usernameOk := common.NormalizeUsername(msg.Payload.Username)
	if !usernameOk {
		return bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, "invalid target username")
	}

	if target == client.Username {
		return bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, "cannot issue a handshake token whose target is yourself")
	}

	room := client.Room
	token := room.TokenManager.NewClientToken(room.Name, client.Username, target)

	return bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_TOKEN, &pb.MsgDirectConnHandshakeToken{
		Token: token,
	})
}
