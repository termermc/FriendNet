package client

import (
	"context"
	"errors"
	"sync"

	"friendnet.org/client/cert"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// ErrRoomConnClosed is returned when trying to interact with a closed room connection.
var ErrRoomConnClosed = errors.New("room connection closed")

// RoomConn represents a room connection.
// The room connection contains a connection to a central server, as well as potentially direct connections with peers in the room.
// A RoomConn is always in an authenticated and usable state until it is closed, either by calling RoomConn.Close, or the connection being interrupted.
type RoomConn struct {
	mu       sync.RWMutex
	isClosed bool

	clientVer *pb.ProtoVersion
	serverVer *pb.ProtoVersion

	// The room's context.
	Context   context.Context
	ctxCancel context.CancelFunc

	serverConn protocol.ProtoConn
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
func authenticate(
	serverConn protocol.ProtoConn,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) error {
	res, err := serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_AUTHENTICATE, &pb.MsgAuthenticate{
		Room:     room.String(),
		Username: username.String(),
		Password: password,
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
	certStore cert.Store,
	address string,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) (*RoomConn, error) {
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
	err = authenticate(conn, room, username, password)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// TODO Read loop

	return &RoomConn{
		clientVer: clientVer,
		serverVer: serverVer,

		Context:   ctx,
		ctxCancel: ctxCancel,

		serverConn: conn,
	}, nil
}

// Close closes the room connection.
// Subsequent calls are no-op.
func (rc *RoomConn) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.isClosed {
		return nil
	}
	rc.isClosed = true

	_ = rc.serverConn.CloseWithReason("closing")

	rc.ctxCancel()

	return nil
}
