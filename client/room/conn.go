package room

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"friendnet.org/client/cert"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
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
type Conn struct {
	mu       sync.RWMutex
	isClosed bool

	logger *slog.Logger

	handlers MessageHandlers

	clientVer *pb.ProtoVersion
	serverVer *pb.ProtoVersion

	// The room name.
	RoomName common.NormalizedRoomName

	// The current user's username.
	Username common.NormalizedUsername

	// The room's context.
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
	handlers MessageHandlers,
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

		handlers: handlers,

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
	go c.s2cLoop()

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

	_ = c.serverConn.CloseWithReason("closing")

	c.ctxCancel()

	return nil
}
