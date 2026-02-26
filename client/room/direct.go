package room

import (
	"context"
	"encoding/base64"
	"errors"
	"hash/fnv"
	"net/netip"
	"unsafe"

	"friendnet.org/client/direct"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

var errEmptyHandshakeToken = errors.New("empty handshake token")

func (c *Conn) redeemDirectHandshakeToken(token string) (*pb.MsgRedeemConnHandshakeTokenResult, error) {
	if token == "" {
		return nil, errEmptyHandshakeToken
	}

	msg, err := protocol.SendAndReceiveExpect[*pb.MsgRedeemConnHandshakeTokenResult](
		c.serverConn,
		pb.MsgType_MSG_TYPE_REDEEM_CONN_HANDSHAKE_TOKEN,
		&pb.MsgRedeemConnHandshakeToken{
			Token: token,
		},
		pb.MsgType_MSG_TYPE_REDEEM_CONN_HANDSHAKE_TOKEN_RESULT,
	)
	if err != nil {
		return nil, err
	}

	return msg.Payload, nil
}

func (c *Conn) mkMethodId(addrPort netip.AddrPort) string {
	addrStr := addrPort.String()
	hasher := fnv.New64a()
	_, _ = hasher.Write(unsafe.Slice(unsafe.StringData(addrStr), len(addrStr)))
	b64 := base64.RawURLEncoding.EncodeToString(hasher.Sum(nil))

	return c.directPart.CreateMethodId(b64)
}

// mkAdConnMethod returns a message that can be used to advertise a direct connection method.
// publicIp will be ignored if invalid/empty.
//
// Priorities:
// 2 = public IP
// 1 = default
// 0 = private IP
// -1 = Yggdrasil
func (c *Conn) mkAdConnMethod(publicIp netip.Addr, addrPort netip.AddrPort) *pb.MsgAdvertiseConnMethod {
	addr := addrPort.Addr()
	isYggdrasil := common.YggdrasilPrefix.Contains(addr)

	var methodType pb.ConnMethodType
	var priority int32
	if isYggdrasil {
		priority = -1
		methodType = pb.ConnMethodType_CONN_METHOD_TYPE_YGGDRASIL
	} else {
		if publicIp.IsValid() && addr == publicIp {
			priority = 2
		} else if addr.IsPrivate() {
			priority = 0
		} else {
			priority = 1
		}

		methodType = pb.ConnMethodType_CONN_METHOD_TYPE_IP
	}

	return &pb.MsgAdvertiseConnMethod{
		Id:       c.mkMethodId(addrPort),
		Type:     methodType,
		Address:  addrPort.String(),
		Priority: priority,
	}
}

func (c *Conn) runDirectAdsAndLoop() {
	mgr := c.directMgr

	if mgr.IsDisabled() {
		return
	}

	var publicIp netip.Addr

	if !mgr.IsPublicIpDiscoveryDisabled() {
		// Ask for public IP from the server and notify the manager of it.
		func() {
			msg, err := protocol.SendAndReceiveExpect[*pb.MsgPublicIp](
				c.serverConn,
				pb.MsgType_MSG_TYPE_GET_PUBLIC_IP,
				&pb.MsgGetPublicIp{},
				pb.MsgType_MSG_TYPE_PUBLIC_IP,
			)
			if err != nil {
				c.logger.Error("failed to get public IP from server",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"err", err,
				)
				return
			}

			publicIp, err = netip.ParseAddr(msg.Payload.PublicIp)
			if err != nil {
				c.logger.Error("failed to parse public IP from server",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"ip", msg.Payload.PublicIp,
					"err", err,
				)
				return
			}

			mgr.NotifyIpAvailable(publicIp)
		}()
	}

	advertiseInBg := func(server *direct.Server) {
		method := c.mkAdConnMethod(publicIp, server.AddrPort)

		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					c.logger.Error("direct advertisement goroutine panicked",
						"service", "room.Conn",
						"room", c.RoomName.String(),
						"addr", server.AddrPort.String(),
						"err", rec,
					)
				}
			}()

			msg, err := protocol.SendAndReceiveExpect[*pb.MsgAdvertiseConnMethodResult](
				c.serverConn,
				pb.MsgType_MSG_TYPE_ADVERTISE_CONN_METHOD,
				method,
				pb.MsgType_MSG_TYPE_ADVERTISE_CONN_METHOD_RESULT,
			)
			if err != nil {
				if protocol.IsErrorConnCloseOrCancel(err) {
					return
				}

				c.logger.Error("failed to advertise direct connection method",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"method_type", method.Type.String(),
					"address", server.AddrPort.String(),
					"priority", method.Priority,
					"err", err,
				)
				return
			}

			result := msg.Payload.TestResult
			if result == pb.ConnResult_CONN_RESULT_OK {
				c.logger.Info("server verified advertised address",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"method_type", method.Type.String(),
					"address", server.AddrPort.String(),
					"priority", method.Priority,
				)
			} else {
				c.logger.Error("server said it could not connect to advertised address",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"method_type", method.Type.String(),
					"address", server.AddrPort.String(),
					"priority", method.Priority,
					"result", result.String(),
				)
			}

			// TODO If ok, record the method in Conn struct.
			// Later on, if we have any verified methods, we can ask a client to connect to us as a direct connect method.
		}()
	}

	// Advertise known servers.
	servers := mgr.GetServers()
	for _, server := range servers {
		advertiseInBg(server)
	}

	// Listen for new direct methods from partition.
	go func() {
		for {
			server, err := c.directPart.WaitServerOpen()
			if err != nil {
				return
			}

			advertiseInBg(server)
		}
	}()

	// Listen for direct methods closing from partition.
	go func() {
		for {
			server, err := c.directPart.WaitServerClose()
			if err != nil {
				return
			}

			mtdId := c.mkMethodId(server.AddrPort)

			// TODO Remove from internal method map.

			// Remove advertisement in background.
			go func() {
				_, err = protocol.SendAndReceiveExpect[*pb.MsgAcknowledged](
					c.serverConn,
					pb.MsgType_MSG_TYPE_REMOVE_CONN_METHOD,
					&pb.MsgRemoveConnMethod{
						Id: mtdId,
					},
					pb.MsgType_MSG_TYPE_ACKNOWLEDGED,
				)
				if err != nil {
					c.logger.Error("failed to remove direct method advertisement",
						"service", "room.Conn",
						"room", c.RoomName.String(),
						"err", err,
						"method_id", mtdId,
					)
					return
				}
			}()
		}
	}()

	// For the rest of the loop, accept direct connections.
	for {
		conn, err := c.directPart.AcceptConn()
		if err != nil {
			if errors.Is(err, direct.ErrPartitionClosed) {
				// No more connections to accept.
				return
			}

			c.logger.Error("failed to accept direct connection",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"err", err,
			)
			continue
		}

		go c.incomingDirectConnHandler(conn)
	}
}

func (c *Conn) incomingDirectConnHandler(incomingConn *direct.IncomingDirectConn) {
	tokenRes, err := c.redeemDirectHandshakeToken(incomingConn.Handshake.Token)
	if err != nil {
		c.logger.Error("failed to redeem direct conn handshake token from incoming direct conn",
			"service", "room.Conn",
			"err", err,
			"token", incomingConn.Handshake.Token,
			"remote_addr", incomingConn.RemoteAddr().String(),
		)
		_ = incomingConn.InternalError()
		return
	}

	if !tokenRes.IsValid {
		_ = incomingConn.InvalidToken()
		return
	}

	if tokenRes.IsServer {
		_ = incomingConn.KThxBye()
		return
	}

	if tokenRes.Room != c.RoomName.String() {
		// How did this even happen?
		c.logger.Error("direct conn handshake token room mismatch",
			"service", "room.Conn",
			"token_room", tokenRes.Room,
			"expected_room", c.RoomName.String(),
			"remote_addr", incomingConn.RemoteAddr().String(),
		)
		_ = incomingConn.InvalidToken()
		return
	}

	username, usernameOk := common.NormalizeUsername(tokenRes.Username)
	if !usernameOk {
		c.logger.Error("server sent invalid username in direct conn handshake token result",
			"service", "room.Conn",
			"username", tokenRes.Username,
			"remote_addr", incomingConn.RemoteAddr().String(),
		)
		_ = incomingConn.InvalidToken()
		return
	}

	c.mu.RLock()

	if c.isClosed {
		c.mu.RUnlock()

		// Client closed between the beginning of the handshake and now.
		_ = incomingConn.KThxBye()
		return
	}
	c.mu.RUnlock()

	conn, err := incomingConn.Approve()
	if err != nil {
		if protocol.IsErrorConnCloseOrCancel(err) {
			return
		}

		c.logger.Error("failed to approve direct connection",
			"service", "room.Conn",
			"room", c.RoomName.String(),
			"username", username.String(),
			"remote_addr", incomingConn.RemoteAddr().String(),
			"err", err,
		)
		return
	}

	// Assign connection to map, getting reference to existing if any.
	c.mu.Lock()
	existing, hasExisting := c.directConns[username]
	c.directConns[username] = conn
	c.mu.Unlock()

	c.logger.Info("client made direct connection",
		"room", c.RoomName.String(),
		"username", username.String(),
		"remote_addr", incomingConn.RemoteAddr().String(),
	)

	if hasExisting {
		// Close existing connection.
		_ = existing.CloseWithReason("new connection from same client")
	}

	// TODO Ping loop

	// Handle authenticated connection.
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				c.logger.Error("direct conn read loop panicked",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"username", username.String(),
					"err", rec,
				)
			}
		}()
		defer func() {
			c.mu.Lock()
			delete(c.directConns, username)
			c.mu.Unlock()

			c.logger.Info("client disconnected from direct connection",
				"room", c.RoomName.String(),
				"username", username.String(),
				"remote_addr", conn.RemoteAddr().String(),
			)
		}()

		loopErr := c.directConnReadLoop(conn, username)
		if loopErr != nil {
			if protocol.IsErrorConnCloseOrCancel(loopErr) {
				return
			}

			c.logger.Error("direct conn read loop exited with error",
				"service", "room.Conn",
				"err", loopErr,
				"room", c.RoomName.String(),
				"username", username.String(),
				"remote_addr", conn.RemoteAddr().String(),
			)
		}
	}()
}

func (c *Conn) directConnReadLoop(conn protocol.ProtoConn, username common.NormalizedUsername) error {
	for {
		bidi, err := conn.WaitForBidi(c.Context)
		if err != nil {
			return err
		}

		c.incomingBidi <- C2cBidi{
			ProtoBidi: bidi,
			RoomConn:  c,
			Username:  username,
		}
	}
}

func (c *Conn) tryDirectConnect(ctx context.Context, username common.NormalizedUsername) (protocol.ProtoConn, error) {
	// First, check if we already have a connection.
	c.mu.RLock()
	existing, hasExisting := c.directConns[username]
	methods, hasMethods := c.directConnMethods[username]
	c.mu.RUnlock()

	if hasExisting {
		return existing, nil
	}

	if hasMethods {
		if len(methods) == 0 {
			// No methods to reach out to the client, do we have any methods for them to connect to us?
			// TODO
		}
	} else {
		// Let's fetch methods for the user.
	}

	return nil, nil
}
