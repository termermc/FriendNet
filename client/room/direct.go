package room

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"strings"
	"time"

	"friendnet.org/client/router"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

const directConnHandshakeTimeout = 30 * time.Second

var errEmptyHandshakeToken = errors.New("empty handshake token")

// DirectConfig is the configuration for making direct connections to clients.
type DirectConfig struct {
	// Whether to disable direct connect entirely.
	// If true, all other fields will be ignored.
	Disable bool

	// The certificate to use for the direct connect server.
	// Required if Disable is not true.
	Cert tls.Certificate

	// The initial addresses to listen on.
	// Each address must be in the format `IPv4:PORT`, `[IPv6]:PORT`, `IP` (IPv6 without port does not need brackets).
	// Must specify at least one.
	// Can use addresses like `0.0.0.0` and `[::]` (with or without port) to listen on all interfaces.
	// Any addresses without a port will have a port assigned to them.
	Addresses []string

	// The default port to use for addresses that do not have a specified port.
	// It will also be the port opened by UPnP.
	//
	// If 0, a random port will be used.
	// Using a random port is not recommended because it will cause port churn across reconnects.
	// Keeping the port consistent across reconnects is useful because external clients will be able to more reliably reach the client.
	//
	// A port >= 1024 is recommended to avoid permission denied errors from the OS.
	DefaultPort uint16

	// Whether to disable probing the machine for IPs to advertise.
	// It does not advertise private IPs unless AdvertisePrivateIps is true.
	DisableProbeIpsToAdvertise bool

	// Whether to advertise private IPs (like 192.168.0.0/16, 172.16.0.0/12, 10.0.0.0/8).
	// Has no effect if ProbeIpsToAdvertise is false.
	// This only makes sense when multiple clients are on the same LAN or VPN.
	AdvertisePrivateIps bool

	// Whether to disable public IP discovery via the server.
	// By default, the client will try to discover its public IP by asking the server for it.
	DisablePublicIpDiscovery bool

	// Whether to disable UPnP.
	DisableUPnP bool

	// The timeout for using UPnP.
	// Defaults to 10 seconds.
	// Has no effect if DisableUPnP is true.
	UpnpTimeout time.Duration
}

// Validate validates a DirectConfig and returns its parsed IP-port values.
// The addresses without ports will have 0 as port.
func (cfg DirectConfig) Validate() (addrs map[netip.AddrPort]struct{}, err error) {
	if cfg.Disable {
		return nil, nil
	}

	if cfg.Cert.Certificate == nil {
		return nil, fmt.Errorf(`missing Cert in DirectConfig`)
	}

	// Validate address formats.
	addrs = make(map[netip.AddrPort]struct{}, len(cfg.Addresses))
	for _, addrStr := range cfg.Addresses {
		if strings.ContainsRune(addrStr, ':') {
			val, parseErr := netip.ParseAddrPort(addrStr)
			if parseErr != nil {
				return nil, fmt.Errorf(`invalid address %q in DirectConfig: %w`, addrStr, parseErr)
			}
			addrs[val] = struct{}{}
		} else {
			addr, parseErr := netip.ParseAddr(addrStr)
			if parseErr != nil {
				return nil, fmt.Errorf(`invalid address %q in DirectConfig: %w`, addrStr, parseErr)
			}
			addrs[netip.AddrPortFrom(addr, 0)] = struct{}{}
		}
	}

	return addrs, nil
}

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

func (c *Conn) startDirectServersAndAds(addrs map[netip.AddrPort]struct{}) {
	// Assumes the caller has already checked that the config is valid and Disable is false.

	defaultPort := c.directCfg.DefaultPort
	if defaultPort == 0 {
		const minPort = 1024
		defaultPort = uint16(rand.IntN(65535-minPort) + minPort)
	}

	var publicIp netip.Addr

	if !c.directCfg.DisableUPnP {
		timeout := c.directCfg.UpnpTimeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}

		timeoutCtx, cancel := context.WithTimeout(c.Context, timeout)

		ipStr, err := router.GetIpAndForwardPort(timeoutCtx, defaultPort)
		if err != nil &&
			!errors.Is(err, context.DeadlineExceeded) &&
			!errors.Is(err, context.Canceled) {
			c.logger.Error("UPnP public IP discovery and forwarding failed",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"err", err,
			)
		}

		publicIp, err = netip.ParseAddr(ipStr)
		if err != nil {
			c.logger.Error("UPnP public IP discovery succeeded, but the public IP it discovered could not be parsed",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"ip", ipStr,
				"err", err,
			)
		}

		cancel()
	}

	if !publicIp.IsValid() {
		// Ask for public IP from the server.
		msg, err := protocol.SendAndReceiveExpect[*pb.MsgPublicIp](
			c.serverConn,
			pb.MsgType_MSG_TYPE_GET_PUBLIC_IP,
			&pb.MsgGetPublicIp{},
			pb.MsgType_MSG_TYPE_PUBLIC_IP,
		)
		if err == nil {
			publicIp, err = netip.ParseAddr(msg.Payload.PublicIp)
			if err != nil {
				c.logger.Error("failed to parse public IP from server",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"ip", msg.Payload.PublicIp,
					"err", err,
				)
			}
		} else {
			c.logger.Error("failed to get public IP from server",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"err", err,
			)
		}
	}

	var probedIps []netip.Addr
	if !c.directCfg.DisableProbeIpsToAdvertise {
		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				ifaceAddrs, addrsErr := iface.Addrs()
				if addrsErr != nil {
					c.logger.Error("failed to get interface addresses",
						"service", "room.Conn",
						"room", c.RoomName.String(),
						"interface", iface.Name,
						"err", addrsErr,
					)
					continue
				}

				for _, oldAddr := range ifaceAddrs {
					addr := netip.MustParseAddr(oldAddr.String())
					if addr.IsPrivate() && !c.directCfg.AdvertisePrivateIps {
						continue
					}

					probedIps = append(probedIps, addr)
				}
			}
		} else {
			c.logger.Error("failed to get network interfaces to discover client IPs",
				"service", "room.Conn",
				"room", c.RoomName.String(),
				"err", err,
			)
		}
	}

	// Collect addresses to listen on and advertise.
	listenAddrPorts := make([]netip.AddrPort, 0, 1+len(addrs)+len(probedIps))
	if publicIp.IsValid() {
		listenAddrPorts = append(listenAddrPorts, netip.AddrPortFrom(publicIp, defaultPort))
	}
	for addrPort := range addrs {
		if addrPort.Port() == 0 {
			listenAddrPorts = append(listenAddrPorts, netip.AddrPortFrom(addrPort.Addr(), defaultPort))
		} else {
			listenAddrPorts = append(listenAddrPorts, addrPort)
		}
	}
	for _, addr := range probedIps {
		addrPort := netip.AddrPortFrom(addr, defaultPort)
		listenAddrPorts = append(listenAddrPorts, addrPort)
	}

	// Start listeners for each address.
	for _, addrPort := range listenAddrPorts {
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					c.logger.Error("direct server run function panicked",
						"service", "room.Conn",
						"room", c.RoomName.String(),
						"addr", addrPort.String(),
						"err", rec,
					)
				}
			}()

			serverErr := c.runDirectServer(addrPort, c.directCfg.Cert)
			if serverErr != nil {
				if protocol.IsErrorConnCloseOrCancel(serverErr) {
					return
				}

				c.logger.Error("error listening on direct server",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"addr", addrPort.String(),
					"err", serverErr,
				)
			}
		}()
	}

	// Advertise every address.
	for _, addrPort := range listenAddrPorts {
		addr := addrPort.Addr()
		isYggdrasil := common.YggdrasilPrefix.Contains(addr)
		var isFromCfg bool
		if _, has := addrs[addrPort]; has {
			isFromCfg = true
		}
		if _, has := addrs[netip.AddrPortFrom(addr, 0)]; has && addrPort.Port() == defaultPort {
			isFromCfg = true
		}

		// Priorities:
		// 3 = address from config
		// 2 = public IP
		// 1 = other (such as discovered from interface)
		// 0 = private IP
		// -1 = Yggdrasil

		var methodType pb.ConnMethodType
		var priority int32
		if isYggdrasil {
			priority = -1
			methodType = pb.ConnMethodType_CONN_METHOD_TYPE_YGGDRASIL
		} else {
			if isFromCfg {
				priority = 3
			} else if publicIp.IsValid() && addr == publicIp {
				priority = 2
			} else if addr.IsPrivate() {
				priority = 0
			} else {
				priority = 1
			}

			methodType = pb.ConnMethodType_CONN_METHOD_TYPE_IP
		}

		method := &pb.MsgAdvertiseConnMethod{
			Type:     methodType,
			Address:  addrPort.String(),
			Priority: priority,
		}

		// Advertise in the background.
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					c.logger.Error("direct advertisement goroutine panicked",
						"service", "room.Conn",
						"room", c.RoomName.String(),
						"addr", addrPort.String(),
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
					"method_type", methodType.String(),
					"address", addrPort.String(),
					"priority", priority,
					"err", err,
				)
				return
			}

			result := msg.Payload.Result
			if result != pb.ConnResult_CONN_RESULT_OK {
				c.logger.Error("server said it could not connect to advertised address",
					"service", "room.Conn",
					"room", c.RoomName.String(),
					"method_type", methodType.String(),
					"address", addrPort.String(),
					"priority", priority,
					"result", result.String(),
				)
			}

			// TODO If ok, record the method.
			// Later on, if we have any verified methods, we can ask a client to connect to us as a direct connect method.
		}()
	}
}

func (c *Conn) runDirectServer(ipPort netip.AddrPort, cert tls.Certificate) error {
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{protocol.AlpnProtoName},
	}
	listener, listenErr := protocol.NewQuicProtoListener(ipPort.String(), tlsCfg)
	if listenErr != nil {
		return fmt.Errorf(`failed to create direct listener on %q: %w`, ipPort.String(), listenErr)
	}

	defer func() {
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept(c.Context)
		if err != nil {
			return fmt.Errorf(`failed to accept direct connection: %w`, err)
		}
		go c.directConnHandler(conn)
	}
}

func (c *Conn) directConnHandler(conn protocol.ProtoConn) {
	isOk := false
	ctx, cancel := context.WithTimeout(c.Context, directConnHandshakeTimeout)
	defer cancel()

	go func() {
		<-ctx.Done()
		if !isOk {
			_ = conn.CloseWithReason("handshake timed out")
		}
	}()

	bidi, waitErr := conn.WaitForBidi(ctx)
	if waitErr != nil {
		if protocol.IsErrorConnCloseOrCancel(waitErr) {
			return
		}

		c.logger.Error("failed to wait for bidi from unauthenticated direct connection",
			"service", "room.Conn",
			"err", waitErr,
			"remote_addr", conn.RemoteAddr().String(),
		)
	}
	defer func() {
		_ = bidi.Close()
	}()

	msg, err := protocol.ReadExpect[*pb.MsgDirectConnHandshake](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE)
	if err != nil {
		var streamErr *quic.StreamError
		if protocol.IsErrorConnCloseOrCancel(err) ||
			errors.As(err, &streamErr) {
			return
		}

		var unexpectedErr protocol.UnexpectedMsgTypeError
		if errors.As(err, &unexpectedErr) {
			c.logger.Error("received unexpected message type during direct conn handshake",
				"service", "room.Conn",
				"expected_type", unexpectedErr.Expected.String(),
				"actual_type", unexpectedErr.Actual.String(),
				"remote_addr", conn.RemoteAddr().String(),
			)
			return
		}

		c.logger.Error("failed to read direct conn handshake message",
			"service", "room.Conn",
			"err", err,
			"remote_addr", conn.RemoteAddr().String(),
		)

		return
	}

	tokenRes, err := c.redeemDirectHandshakeToken(msg.Payload.Token)
	if err != nil {
		c.logger.Error("failed to redeem direct conn handshake token from unauthenticated direct conn handshake",
			"service", "room.Conn",
			"err", err,
			"token", msg.Payload.Token,
			"remote_addr", conn.RemoteAddr().String(),
		)
		return
	}

	if !tokenRes.IsValid {
		// Bye.
		_ = bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
			Result: pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_TOKEN_INVALID,
		})
		_ = bidi.Close()
		_ = conn.CloseWithReason("invalid token")
		return
	}

	if tokenRes.IsServer {
		_ = bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
			Result: pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_KTHXBYE,
		})
		_ = bidi.Close()
		_ = conn.CloseWithReason("server handshake succeeded")
		return
	}

	if tokenRes.Room != c.RoomName.String() {
		// How did this even happen?
		c.logger.Error("direct conn handshake token room mismatch",
			"service", "room.Conn",
			"token_room", tokenRes.Room,
			"expected_room", c.RoomName.String(),
			"remote_addr", conn.RemoteAddr().String(),
		)
		_ = bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
			Result: pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_TOKEN_INVALID,
		})
		_ = bidi.Close()
		_ = conn.CloseWithReason("invalid token")
		return
	}

	username, usernameOk := common.NormalizeUsername(tokenRes.Username)
	if !usernameOk {
		c.logger.Error("server sent invalid username in direct conn handshake token result",
			"service", "room.Conn",
			"username", tokenRes.Username,
			"remote_addr", conn.RemoteAddr().String(),
		)
		_ = bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
			Result: pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_TOKEN_INVALID,
		})
		_ = bidi.Close()
		_ = conn.CloseWithReason("invalid token")
		return
	}

	isOk = true

	c.mu.Lock()

	if c.isClosed {
		c.mu.Unlock()

		// Client closed between the beginning of the handshake and now.
		_ = bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
			Result: pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_KTHXBYE,
		})
		_ = bidi.Close()
		_ = conn.CloseWithReason("token valid, but client is closed")
		return
	}

	// Assign connection to map, getting reference to existing if any.
	existing, hasExisting := c.directConns[username]
	c.directConns[username] = conn
	c.mu.Unlock()

	c.logger.Info("client made direct connection",
		"room", c.RoomName.String(),
		"username", username.String(),
		"remote_addr", conn.RemoteAddr().String(),
	)

	if hasExisting {
		// Close existing connection.
		_ = existing.CloseWithReason("new connection from same client")
	}

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
}
