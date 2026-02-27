package room

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"friendnet.org/client/cert"
	"friendnet.org/client/direct"
	"friendnet.org/common"
	"friendnet.org/common/machine"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

// ServerPingInterval is the interval between pings sent to the server.
const ServerPingInterval = 10 * time.Second

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

	logic             Logic
	connMethodSupport machine.ConnMethodSupport

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

	// The direct server manager.
	directMgr *direct.Manager

	// The direct connect server partition.
	directPart *direct.Partition

	// Direct connections to room clients.
	// The connections could have been outgoing or incoming,
	// but they are treated the same once established.
	directConns map[common.NormalizedUsername]protocol.ProtoConn

	// A cache of direct connect methods for room clients.
	// If no methods are available for a client, the slice will be empty.
	// If we have not checked methods for a client yet, there will be no value for that client.
	// Cleared periodically.
	directPeerMethods map[common.NormalizedUsername][]*pb.ConnMethod

	// A map of direct connect methods for the client.
	// They may be verified by the server, they may not.
	directSelfMethods map[string]*pb.ConnMethod

	// A set of usernames that we have failed to directly connect to.
	// Cleared periodically.
	directConnectOutgoingFailures map[common.NormalizedUsername]struct{}

	// A set of usernames that failed to directly connect to us when we sent them CONNECT_TO_ME.
	// Cleared periodically.
	directConnectToMeFailures map[common.NormalizedUsername]struct{}

	// The timeout for establishing outgoing direct connections.
	directOutgoingTimeout time.Duration

	// The interval at which direct connection-related caches are cleared.
	directGcInterval time.Duration
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
//
// The directPartitionName value must be unique among open Conn instances that use the same direct.Manager.
// It could be a server UUID, or something else unique to the connection.
// If an open Conn instance has the name "abc" and this function is called with directPartitionName "abc", it will return an error.
func NewRoomConn(
	logger *slog.Logger,
	logic Logic,
	connMethodSupport machine.ConnMethodSupport,
	certStore cert.Store,
	directMgr *direct.Manager,
	directPartitionName string,
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

	directPart, err := directMgr.CreatePartition(directPartitionName)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	c := &Conn{
		logger: logger,

		logic:             logic,
		connMethodSupport: connMethodSupport,

		clientVer: clientVer,
		serverVer: serverVer,

		RoomName: creds.Room,
		Username: creds.Username,

		Context:   ctx,
		ctxCancel: ctxCancel,

		serverConn:   conn,
		incomingBidi: make(chan C2cBidi, incomingBidiChanSize),

		directMgr:                     directMgr,
		directPart:                    directPart,
		directConns:                   make(map[common.NormalizedUsername]protocol.ProtoConn),
		directPeerMethods:             make(map[common.NormalizedUsername][]*pb.ConnMethod),
		directSelfMethods:             make(map[string]*pb.ConnMethod),
		directConnectOutgoingFailures: make(map[common.NormalizedUsername]struct{}),
		directConnectToMeFailures:     make(map[common.NormalizedUsername]struct{}),
		directOutgoingTimeout:         10 * time.Second,
		directGcInterval:              5 * time.Minute,
	}

	go c.directCacheGc()

	go c.c2cLoop()

	go func() {
		c.s2cLoop()

		// S2C loop exited, so the server must have gone away.
		// Close connection.
		_ = c.Close()
	}()
	go func() {
		c.pingLoop()

		// Ping loop exited, so the server must have gone away.
		// Close connection.
		_ = c.Close()
	}()

	go c.runDirectAdsAndLoop()

	return c, nil
}

// Ping sends a ping request to the client and returns the round-trip time.
func (c *Conn) Ping() (time.Duration, error) {
	start := time.Now()
	_, err := c.serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
		SentTs: start.UnixMilli(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to send ping to server: %w", err)
	}

	return time.Since(start), nil
}

func (c *Conn) pingLoop() {
	ticker := time.NewTicker(ServerPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Context.Done():
			return
		case <-ticker.C:
			if _, err := c.Ping(); err != nil {
				if protocol.IsErrorConnCloseOrCancel(err) {
					return
				}

				c.logger.Error("failed to ping server",
					"service", "room.Conn",
					"err", err,
				)
			}
			_, _ = c.serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{})
		}
	}
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

	_ = c.directPart.Close()

	// Signal to the server that the client is leaving.
	// Give it 5 seconds to respond before closing the connection.
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		_, _ = c.serverConn.SendAndReceive(pb.MsgType_MSG_TYPE_BYE, &pb.MsgBye{})
		cancel()
	}()
	<-timeoutCtx.Done()

	_ = c.serverConn.CloseWithReason("goodbye")

	c.ctxCancel()

	return nil
}

func (c *Conn) isErrProxyPeerUnreachable(err error) bool {
	if streamErr, ok := errors.AsType[*quic.StreamError](err); ok {
		return streamErr.ErrorCode == protocol.ProxyPeerUnreachableStreamErrorCode
	}
	return false
}

// openC2cBidi opens a proxied bidi to a destination peer.
// If the peer is definitely unreachable, returns protocol.ErrPeerUnreachable.
// The caller should defensively call isErrProxyPeerUnreachable on the error checks for bidi reads.
func (c *Conn) openProxiedC2cBidi(username common.NormalizedUsername) (protocol.ProtoBidi, error) {
	bidi, err := c.serverConn.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_OPEN_OUTBOUND_PROXY, &pb.MsgOpenOutboundProxy{
		TargetUsername: username.String(),
	})
	if err != nil {
		if c.isErrProxyPeerUnreachable(err) {
			return protocol.ProtoBidi{}, protocol.ErrPeerUnreachable
		}
		if _, ok := errors.AsType[*quic.ApplicationError](err); ok {
			// Other side closed, and we didn't know about it until now.
			_ = c.Close()
			return protocol.ProtoBidi{}, ErrRoomConnClosed
		}

		return protocol.ProtoBidi{}, err
	}

	return bidi, nil
}

// openC2cBidiWithMsg opens a bidi to a destination peer.
// If the peer is definitely unreachable, returns protocol.ErrPeerUnreachable.
// It may not return an error if the peer is unreachable immediately, but read methods
// will most likely return protocol.ErrPeerUnreachable later if the peer is unreachable.
//
// If forceProxy is true, it will always proxy to the peer instead of using a direct connection.
// It may still fall back to proxying if no direct connection method is available.
func (c *Conn) openC2cBidiWithMsg(
	username common.NormalizedUsername,
	typ pb.MsgType,
	msg proto.Message,
	forceProxy bool,
) (protocol.ProtoBidi, error) {
	var directConn protocol.ProtoConn
	if !forceProxy && !c.directMgr.IsDisabled() {
		// Collect information that will be useful for helping us connect.
		c.mu.RLock()
		existing, hasExisting := c.directConns[username]
		selfMethods := make([]*pb.ConnMethod, 0, len(c.directSelfMethods))
		for _, method := range c.directSelfMethods {
			selfMethods = append(selfMethods, method)
		}
		_, hasFailedConnectToMe := c.directConnectToMeFailures[username]
		_, hasFailedOutgoing := c.directConnectOutgoingFailures[username]
		c.mu.RUnlock()

		// Are we already connected?
		if hasExisting {
			directConn = existing
			goto openBidi
		}

		timeoutCtx, ctxCancel := context.WithTimeout(c.Context, c.directOutgoingTimeout)
		defer ctxCancel()

		var connErr error

		// Have we already tried and failed to connect to this peer?
		if hasFailedOutgoing {
			goto tryConnectToMe
		}

		// Try to connect directly.
		directConn, _, connErr = c.tryConnectToPeerAndAddToMap(timeoutCtx, username)
		if connErr != nil {
			// Was the client not online?
			if protoErr, ok := errors.AsType[*protocol.ProtoMsgError](connErr); ok {
				if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_CLIENT_NOT_ONLINE {
					// The client was offline.
					// Just return the error as-is.
					// Do not cache failure.
					return protocol.ProtoBidi{}, connErr
				}
			}

			// Record this failure.
			c.mu.Lock()
			c.directConnectOutgoingFailures[username] = struct{}{}
			c.mu.Unlock()

			if errors.Is(connErr, errNoPeerMethods) {
				// No peer methods.
				// Try to have the peer connect to us.
				goto tryConnectToMe
			}

			c.logger.Warn("all methods failed to connect to peer",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"peer", username.String(),
				"err", connErr,
			)

			// Oh well.
			// Let's try to have the peer connect to us.
			goto tryConnectToMe
		}

	tryConnectToMe:

		// The heuristic follows these steps in order:
		//  - Do we have a cached CONNECT_TO_ME failure? If so, proxy.
		//  - Do we have any verified IP methods? If so, CONNECT_TO_ME.
		//  - Do we have any Yggdrasil methods? If so, CONNECT_TO_ME.
		//  - If none of the above, proxy.

		if hasFailedConnectToMe {
			// The client tried and failed to connect to us before.
			// Fall back to proxy.
			goto openBidi
		}

		for _, method := range selfMethods {
			if method.Type == pb.ConnMethodType_CONN_METHOD_TYPE_IP && method.IsServerVerified {
				goto connectToMe
			}
			if method.Type == pb.ConnMethodType_CONN_METHOD_TYPE_YGGDRASIL {
				goto connectToMe
			}
		}

		// No suitable self method found, otherwise would have jumped to connectToMe.
		// Fall back to proxy.
		goto openBidi

	connectToMe:
		// Ask the peer to connect to us.
		bidi, err := c.openProxiedC2cBidi(username)
		if err != nil {
			return protocol.ProtoBidi{}, err
		}
		err = bidi.Write(pb.MsgType_MSG_TYPE_CONNECT_TO_ME, &pb.MsgConnectToMe{})
		if err != nil {
			if c.isErrProxyPeerUnreachable(err) {
				return protocol.ProtoBidi{}, protocol.ErrPeerUnreachable
			}
			_ = bidi.Close()
			return protocol.ProtoBidi{}, err
		}
		ctmRes, err := protocol.ReadExpect[*pb.MsgDirectConnResult](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_DIRECT_CONN_RESULT)
		if err != nil {
			if c.isErrProxyPeerUnreachable(err) {
				return protocol.ProtoBidi{}, protocol.ErrPeerUnreachable
			}
			_ = bidi.Close()
			return protocol.ProtoBidi{}, err
		}
		if ctmRes.Payload.Result != pb.ConnResult_CONN_RESULT_OK {
			_ = bidi.Close()

			// The peer could not connect.
			// Record this to save time later.
			c.mu.Lock()
			c.directConnectToMeFailures[username] = struct{}{}
			c.mu.Unlock()
			c.logger.Warn("we asked a peer to connect to us, but it could not",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"peer", username.String(),
				"result", ctmRes.Payload.Result.String(),
			)

			// Fall back to proxy.
			goto openBidi
		}

		// The peer said they were able to connect to us.
		// Check if the connection is active.
		c.mu.RLock()
		existing, hasExisting = c.directConns[username]
		c.mu.RUnlock()
		if !hasExisting {
			c.logger.Warn("a peer said they connected to us, but no connection was found",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"peer", username.String(),
			)

			// Fall back to proxy.
			goto openBidi
		}

		// The peer successfully connected to us!
		directConn = existing
		goto openBidi
	}

openBidi:
	var bidi protocol.ProtoBidi
	var err error
	if directConn == nil {
		bidi, err = c.openProxiedC2cBidi(username)
		if err != nil {
			return protocol.ProtoBidi{}, err
		}
	} else {
		bidi, err = directConn.OpenBidiWithMsg(typ, msg)
		if err != nil {
			return protocol.ProtoBidi{}, err
		}
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
//
// If forceProxy is true, it will always proxy to the peer instead of using a direct connection.
// It may still fall back to proxying if no direct connection method is available.
func (c *Conn) GetVirtualC2cConn(peer common.NormalizedUsername, forceProxy bool) VirtualC2cConn {
	return VirtualC2cConn{
		ServerConn: c,
		Username:   peer,
		ForceProxy: forceProxy,
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
