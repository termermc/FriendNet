package room

import (
	"context"
	"log/slog"
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

	// OnAdvertiseConnMethod handles an incoming advertise connection method request.
	// Implementations must follow the documentation on MSG_TYPE_ADVERTISE_CONN_METHOD.
	OnAdvertiseConnMethod(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgAdvertiseConnMethod],
	) error

	// OnRemoveConnMethod handles an incoming remove connection method request.
	// Implementations must follow the documentation on MSG_TYPE_REMOVE_CONN_METHOD.
	OnRemoveConnMethod(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgRemoveConnMethod],
	) error

	// OnGetPublicIp handles an incoming get public IP request.
	// Implementations must follow the documentation on MSG_TYPE_GET_PUBLIC_IP.
	OnGetPublicIp(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgGetPublicIp],
	) error

	// OnGetClientConnMethods handles an incoming get client connection methods request.
	// Implementations must follow the documentation on MSG_TYPE_GET_CLIENT_CONN_METHODS.
	OnGetClientConnMethods(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgGetClientConnMethods],
	) error

	// OnGetDirectConnHandshakeToken handles an incoming get direct connection handshake token request.
	// Implementations must follow the documentation on MSG_TYPE_GET_DIRECT_CONN_HANDSHAKE_TOKEN.
	OnGetDirectConnHandshakeToken(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgGetDirectConnHandshakeToken],
	) error

	// OnRedeemConnHandshakeToken handles an incoming redeem direct connection handshake token request.
	// Implementations must follow the documentation on MSG_TYPE_REDEEM_CONN_HANDSHAKE_TOKEN.
	OnRedeemConnHandshakeToken(
		ctx context.Context,
		client *Client,
		bidi protocol.ProtoBidi,
		msg *protocol.TypedProtoMsg[*pb.MsgRedeemConnHandshakeToken],
	) error
}

type LogicImpl struct {
	logger *slog.Logger

	directConnTestTimeout time.Duration
}

var _ Logic = (*LogicImpl)(nil)

func NewLogicImpl(logger *slog.Logger) *LogicImpl {
	return &LogicImpl{
		logger: logger,

		directConnTestTimeout: 10 * time.Second,
	}
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

func (l LogicImpl) OnAdvertiseConnMethod(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgAdvertiseConnMethod]) error {
	ad := msg.Payload

	// Validate address.
	if err := protocol.ValidateMethodAddress(ad.Type, ad.Address); err != nil {
		return bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, err.Error())
	}

	// Try to connect.
	connRes := func() pb.ConnResult {
		if !client.Room.connMethodSupport.IsSupported(ad.Type) {
			return pb.ConnResult_CONN_RESULT_METHOD_NOT_SUPPORTED
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, l.directConnTestTimeout)
		defer cancel()

		token := client.Room.TokenManager.NewServerToken()

		conn, result, _ := protocol.CreateDirectConnection(timeoutCtx, ad.Type, ad.Address, &pb.MsgDirectConnHandshake{
			MethodId: ad.Id,
			Token:    token,
		})
		if conn != nil {
			_ = conn.CloseWithReason("direct connection verified")
		}

		return result
	}()

	mtd := &pb.ConnMethod{
		Id:               ad.Id,
		Type:             ad.Type,
		Address:          ad.Address,
		Priority:         ad.Priority,
		IsServerVerified: connRes == pb.ConnResult_CONN_RESULT_OK,
	}

	client.mu.Lock()

	// First, check if this is a duplicate.
	_, has := client.connMethods[ad.Id]
	if has {
		client.mu.Unlock()
		return bidi.Write(pb.MsgType_MSG_TYPE_ADVERTISE_CONN_METHOD_RESULT, &pb.MsgAdvertiseConnMethodResult{
			AlreadyExists: true,
		})
	}

	// Not a duplicate, add method.
	client.connMethods[ad.Id] = mtd
	client.mu.Unlock()

	return bidi.Write(pb.MsgType_MSG_TYPE_ADVERTISE_CONN_METHOD_RESULT, &pb.MsgAdvertiseConnMethodResult{
		AlreadyExists: false,
		TestResult:    connRes,
	})
}

func (l LogicImpl) OnRemoveConnMethod(_ context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgRemoveConnMethod]) error {
	client.mu.Lock()
	_, has := client.connMethods[msg.Payload.Id]
	if !has {
		client.mu.Unlock()
		return bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, "no such method")
	}
	delete(client.connMethods, msg.Payload.Id)
	client.mu.Unlock()

	return bidi.WriteAck()
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

func (l LogicImpl) OnGetClientConnMethods(_ context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetClientConnMethods]) error {
	username, usernameOk := common.NormalizeUsername(msg.Payload.Username)
	if !usernameOk {
		return bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, "invalid username")
	}

	client, has := client.Room.GetClientByUsername(username)
	if !has {
		return bidi.WriteClientNotOnlineError(username)
	}

	return bidi.Write(pb.MsgType_MSG_TYPE_CLIENT_CONN_METHODS, &pb.MsgClientConnMethods{
		Methods: client.GetConnMethods(),
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

func (l LogicImpl) OnRedeemConnHandshakeToken(_ context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgRedeemConnHandshakeToken]) error {
	return bidi.Write(
		pb.MsgType_MSG_TYPE_REDEEM_CONN_HANDSHAKE_TOKEN_RESULT,
		client.Room.TokenManager.Redeem(client.Username, msg.Payload.Token),
	)
}
