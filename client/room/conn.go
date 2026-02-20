package room

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"friendnet.org/client/cert"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

// ErrRoomConnClosed is returned when trying to interact with a closed room connection.
var ErrRoomConnClosed = errors.New("room connection closed")

// Credentials are the credentials used to authenticate with a room.
type Credentials struct {
	// The room name.
	Room common.NormalizedRoomName

	// The room user's username.
	Username common.NormalizedUsername

	// The room user's password.
	Password string
}

// Arbitrary size to prevent lockups on the incoming bidi channel.
const incomingBidiChanSize = 64

// Conn represents a room connection.
// The room connection contains a connection to a central server, as well as potentially direct connections with peers in the room.
// A Conn is always in an authenticated and usable state until it is closed, either by calling RoomConn.Close, or the connection being interrupted.
//
// Important: Does not close the Logic passed to it.
// The lifecycle of its Logic must be managed at a higher level.
// The reason for this is avoiding reinstantiating Logic on reconnects.
type Conn struct {
	mu       sync.RWMutex
	isClosed bool

	logger *slog.Logger

	logic Logic

	clientVer *pb.ProtoVersion
	serverVer *pb.ProtoVersion

	// The room name.
	RoomName common.NormalizedRoomName

	// The current user's username.
	Username common.NormalizedUsername

	// The room's context.
	// Done when the connection is closed.
	Context   context.Context
	ctxCancel context.CancelFunc

	serverConn   protocol.ProtoConn
	incomingBidi chan C2cBidi
}

// negotiateVersion negotiates the protocol version with the server.
// Returns the server's protocol version if successful.
// Returns a protocol.VersionRejectedError if the server rejected the client's version.
func negotiateVersion(serverConn protocol.ProtoConn, clientVer *pb.ProtoVersion) (*pb.ProtoVersion, error) {
	res, err := serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_VERSION, &pb.MsgVersion{
		Version: clientVer,
	})
	if err != nil {
		return nil, err
	}

	switch payload := res.Payload.(type) {
	case *pb.MsgVersionAccepted:
		return payload.Version, nil
	case *pb.MsgVersionRejected:
		return nil, protocol.VersionRejectedError{
			Reason:  payload.Reason,
			Message: common.StrPtrOr(payload.Message, ""),
		}
	default:
		return nil, protocol.NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_VERSION_ACCEPTED, res.Type)
	}
}

// authenticate authenticates with the server.
// Returns a protocol.AuthRejectedError if the server rejected the request.
func authenticate(serverConn protocol.ProtoConn, creds Credentials) error {
	res, err := serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_AUTHENTICATE, &pb.MsgAuthenticate{
		Room:     creds.Room.String(),
		Username: creds.Username.String(),
		Password: creds.Password,
	})
	if err != nil {
		return err
	}

	switch payload := res.Payload.(type) {
	case *pb.MsgAuthAccepted:
		return nil
	case *pb.MsgAuthRejected:
		return protocol.AuthRejectedError{
			Reason:  payload.Reason,
			Message: common.StrPtrOr(payload.Message, ""),
		}
	default:
		return protocol.NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, res.Type)
	}
}

// NewRoomConn establishes a room connection.
// If the server rejects the client's protocol version, returns a protocol.VersionRejectedError.
// If the server rejects the client's credentials, returns a protocol.AuthRejectedError.
func NewRoomConn(
	logger *slog.Logger,
	logic Logic,
	certStore cert.Store,
	address string,
	creds Credentials,
) (*Conn, error) {
	clientVer := protocol.CurrentProtocolVersion

	ctx, ctxCancel := context.WithCancel(context.Background())
	conn, err := ConnectWithCertStore(ctx, certStore, address)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	serverVer, err := negotiateVersion(conn, clientVer)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	err = authenticate(conn, creds)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	c := &Conn{
		logger: logger,

		logic: logic,

		clientVer: clientVer,
		serverVer: serverVer,

		RoomName: creds.Room,
		Username: creds.Username,

		Context:   ctx,
		ctxCancel: ctxCancel,

		serverConn:   conn,
		incomingBidi: make(chan C2cBidi, incomingBidiChanSize),
	}

	go c.c2cLoop()

	go func() {
		c.s2cLoop()

		// S2C loop exited, so the server must have gone away.
		// Close connection.
		_ = c.Close()
	}()

	return c, nil
}

// Close closes the room connection.
// Subsequent calls are no-op.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return nil
	}
	c.isClosed = true

	// Signal to the server that the client is leaving.
	// Give it 5 seconds to respond before closing the connection.
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		_, _ = c.serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_BYE, &pb.MsgBye{})
		cancel()
	}()
	<-timeoutCtx.Done()

	_ = c.serverConn.CloseWithReason("closing")

	c.ctxCancel()

	return nil
}

// openC2cBidiWithMsg opens a bidi to a destination peer.
// If the peer is definitely unreachable, returns protocol.ErrPeerUnreachable.
// It may not return an error if the peer is unreachable immediately, but read methods
// will most likely return protocol.ErrPeerUnreachable later if the peer is unreachable.
func (c *Conn) openC2cBidiWithMsg(
	username common.NormalizedUsername,
	typ pb.MsgType,
	msg proto.Message,
) (protocol.ProtoBidi, error) {
	// We currently do not support direct connections, so we proxy for now.
	bidi, err := c.serverConn.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_OPEN_OUTBOUND_PROXY, &pb.MsgOpenOutboundProxy{
		TargetUsername: username.String(),
	})
	if err != nil {
		var streamErr *quic.StreamError
		var appErr *quic.ApplicationError
		if errors.As(err, &streamErr) {
			if streamErr.ErrorCode == protocol.ProxyPeerUnreachableStreamErrorCode {
				return protocol.ProtoBidi{}, protocol.ErrPeerUnreachable
			}
		}
		if errors.As(err, &appErr) {
			// Other side closed, and we didn't know about it until now.
			_ = c.Close()
			return protocol.ProtoBidi{}, ErrRoomConnClosed
		}

		return protocol.ProtoBidi{}, err
	}

	err = bidi.Write(typ, msg)
	if err != nil {
		_ = bidi.Close()
		return protocol.ProtoBidi{}, err
	}

	return bidi, nil
}

// GetVirtualC2cConn returns a virtual connection to a peer.
// It does not perform any connection logic, it is simply an adapter for the peer
// logic inside Conn.
//
// Methods on the returned VirtualC2cConn may return protocol.ErrPeerUnreachable if the
// desired peer is unavailable.
func (c *Conn) GetVirtualC2cConn(peer common.NormalizedUsername) VirtualC2cConn {
	return VirtualC2cConn{
		ServerConn: c,
		Username:   peer,
	}
}

// GetOnlineUsers returns a stream of online users.
func (c *Conn) GetOnlineUsers() (protocol.Stream[*pb.MsgOnlineUsers], error) {
	bidi, err := c.serverConn.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_GET_ONLINE_USERS, &pb.MsgGetOnlineUsers{})
	if err != nil {
		return nil, err
	}

	return protocol.NewTransformerStream(
		protocol.NewTypedMsgStream[*pb.MsgOnlineUsers](bidi, pb.MsgType_MSG_TYPE_ONLINE_USERS),
		func(msg *protocol.TypedProtoMsg[*pb.MsgOnlineUsers]) *pb.MsgOnlineUsers {
			return msg.Payload
		},
	), nil
}
