package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"friendnet.org/common"
	"friendnet.org/common/machine"
	"friendnet.org/common/password"
	"friendnet.org/protocol"
	"friendnet.org/server/config"
	"friendnet.org/server/lobby"
	"friendnet.org/server/room"
	"friendnet.org/server/storage"
	"friendnet.org/stun"
	"github.com/quic-go/quic-go"
)

// Server is a FriendNet server.
//
// A FriendNet server contains rooms, each one with its own accounts and isolated environment.
// Before entering a room, each new connection is sent to the lobby, where version negotiation and
// authentication are performed. Once the connection is authenticated, it is sent to the
// appropriate room.
type Server struct {
	mu       sync.Mutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	logger    *slog.Logger
	storage   *storage.Storage
	lobby     *lobby.Lobby
	stunAddrs []string

	// The server's room.Manager instance.
	// Do not update or close it.
	RoomManager *room.Manager
}

// NewServer creates a new FriendNet server.
// It uses the specified storage instance.
// It does not start listening until Listen is called.
// Note that Server.Close does not close the storage instance.
func NewServer(
	logger *slog.Logger,
	storage *storage.Storage,
	connMethodSupport machine.ConnMethodSupport,
	passReqs password.Requirements,
	cfg *config.ServerConfig,
) (*Server, error) {
	if storage == nil {
		panic("storage cannot be nil")
	}

	// Before doing anything, we need to figure out the server's STUN addresses.
	var stunAddrs []string
	if len(cfg.StunServers) == 0 {
		// There are no configured STUN server addresses, so we'll try to guess one based on listen addresses.

		stunAddrs = make([]string, 0, len(cfg.Listen))

		wildcardPorts := make(map[uint16]struct{})
		for _, addrStr := range cfg.Listen {
			addrPort, err := netip.ParseAddrPort(addrStr)
			if err != nil {
				host, _, _ := net.SplitHostPort(addrStr)
				if strings.ToLower(host) == "localhost" || strings.HasSuffix(host, ".localhost") {
					// We know that localhost addresses aren't public lol
					continue
				}

				// Not an IP, assume it's a hostname and add it to the list.
				stunAddrs = append(stunAddrs, addrStr)
				continue
			}

			addr := addrPort.Addr()

			// Is this a wildcard address?
			if addr.IsUnspecified() {
				wildcardPorts[addrPort.Port()] = struct{}{}
				continue
			}

			// Is this a normal IP address?
			if !addr.IsPrivate() && !addr.IsLoopback() {
				// Normal IP addresses can be used
				stunAddrs = append(stunAddrs, addrStr)
				continue
			}
		}

		// If there are any wildcard ports, see if we can find a public IPv4 in the machine's interfaces.
		if len(wildcardPorts) > 0 {
			unicastIps := common.GetUnicastIpsFromInterfaces(false, false)
			for _, ip := range unicastIps {
				ip = ip.Unmap()

				// For now, don't support IPv6.
				if ip.Is6() {
					continue
				}

				for port := range wildcardPorts {
					stunAddrs = append(stunAddrs, ip.String()+":"+strconv.Itoa(int(port)))
				}
			}
		}

		if len(stunAddrs) == 0 {
			logger.Warn("no STUN servers provided in server config, and the server's public IP could not be guessed! clients will not be able to use NAT hole punching on this server! see " + common.CfgStunDocsUrl)
		} else {
			logger.Warn("no STUN servers provided in server config, using guessed public address(es)! see "+common.CfgStunDocsUrl, "addrs", strings.Join(stunAddrs, ", "))
		}
	} else {
		stunAddrs = cfg.StunServers
	}

	// Resolve authentication providers.
	accountAuth := lobby.NewAccountAuthProvider(storage)
	var authFunc lobby.AuthFunc
	if cfg.ExternalAuth == nil {
		// Just use built-in auth.
		authFunc = func(
			ctx context.Context,
			ip netip.Addr,
			room common.NormalizedRoomName,
			username common.NormalizedUsername,
			password string,
		) (ok bool, reason string, err error) {
			res, err := accountAuth.Authenticate(ctx, ip, room, username, password)
			if err != nil {
				return false, "", err
			}

			if res.Status == lobby.AuthStatusPass {
				return false, "", fmt.Errorf(`BUG: AccountAuthProvider returned "pass" status`)
			}

			return res.Status == lobby.AuthStatusOk, res.Reason, nil
		}
	} else {
		ext := cfg.ExternalAuth

		// Resolve per-room.
		// We add the global providers to the slice to avoid more lookup logic in the handler.
		perRoomProvs := make(map[common.NormalizedRoomName][]lobby.AuthProvider, len(ext.Rooms))
		for roomNameRaw, roomCfg := range ext.Rooms {
			roomName := common.UncheckedCreateNormalizedRoomName(roomNameRaw)

			provs := make([]lobby.AuthProvider, 0, len(ext.Global)+len(roomCfg.Providers)+1)
			if roomCfg.BeforeGlobal {
				for _, prov := range roomCfg.Providers {
					inst, err := lobby.ProviderFromConfig(prov)
					if err != nil {
						return nil, err
					}
					provs = append(provs, inst)
				}
				for _, prov := range ext.Global {
					inst, err := lobby.ProviderFromConfig(prov)
					if err != nil {
						return nil, err
					}
					provs = append(provs, inst)
				}
			} else {
				for _, prov := range ext.Global {
					inst, err := lobby.ProviderFromConfig(prov)
					if err != nil {
						return nil, err
					}
					provs = append(provs, inst)
				}
				for _, prov := range roomCfg.Providers {
					inst, err := lobby.ProviderFromConfig(prov)
					if err != nil {
						return nil, err
					}
					provs = append(provs, inst)
				}
			}
			provs = append(provs, accountAuth)

			perRoomProvs[roomName] = provs
		}

		// Resolve global.
		globalProvs := make([]lobby.AuthProvider, 0, len(ext.Global)+1)
		for _, prov := range ext.Global {
			inst, err := lobby.ProviderFromConfig(prov)
			if err != nil {
				return nil, err
			}
			globalProvs = append(globalProvs, inst)
		}
		globalProvs = append(globalProvs, accountAuth)

		// Create auth function from these providers.
		authFunc = func(
			ctx context.Context,
			ip netip.Addr,
			room common.NormalizedRoomName,
			username common.NormalizedUsername,
			password string,
		) (ok bool, reason string, err error) {
			var provs []lobby.AuthProvider
			if provs, ok = perRoomProvs[room]; !ok {
				provs = globalProvs
			}

			for _, prov := range provs {
				res, err := prov.Authenticate(ctx, ip, room, username, password)
				if err != nil {
					return false, "", err
				}

				if res.Status == lobby.AuthStatusPass {
					continue
				}

				return res.Status == lobby.AuthStatusOk, res.Reason, nil
			}

			return false, "", fmt.Errorf(`BUG: all providers returned "pass", but built-in should have returned a concrete status`)
		}
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	roomMgr, err := room.NewManager(
		ctx,
		logger,
		storage,
		connMethodSupport,
		passReqs,
		room.NewLogicImpl(logger, cfg, stunAddrs),
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	l := lobby.NewLobby(
		logger,
		authFunc,
		roomMgr,
		lobby.DefaultTimeout,
		protocol.CurrentProtocolVersion,
	)

	return &Server{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		logger:    logger,
		storage:   storage,
		lobby:     l,
		stunAddrs: stunAddrs,

		RoomManager: roomMgr,
	}, nil
}

// Close closes the server.
// Subsequent calls are no-op.
func (s *Server) Close() error {
	s.mu.Lock()

	if s.isClosed {
		s.mu.Unlock()
		return nil
	}
	s.isClosed = true

	s.mu.Unlock()

	_ = s.RoomManager.Close()

	s.ctxCancel()

	return nil
}

// ListenWith starts listening with the specified listener.
// This function can be called concurrently with other listeners to listen on multiple interfaces.
// Returns nil when Server.Close is called.
//
// Does not close the listener.
//
// Use Listen instead if you want to use the default listener.
func (s *Server) ListenWith(listener protocol.ProtoListener) error {
	for {
		conn, err := listener.Accept(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		s.lobby.Onboard(conn)
	}
}

// Listen starts listening on the specified address.
// The address must be in HOST:PORT format, e.g. "127.0.0.1:20038".
// IPv6 addresses must be enclosed in square brackets, e.g. "[::1]:20038".
// This function can be called concurrently with other listeners to listen on multiple interfaces.
// Returns nil when Server.Close is called.
// If runStun is true, the STUN server will be run on the same address and port.
func (s *Server) Listen(address string, tlsCfg *tls.Config, runStun bool) error {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf(`failed to parse listen address %q: %w`, address, err)
	}

	var udpConn *net.UDPConn
	addr := addrPort.Addr()
	if addr.Is6() {
		udpConn, err = net.ListenUDP("udp6", &net.UDPAddr{
			IP:   addr.AsSlice(),
			Port: int(addrPort.Port()),
		})
	} else {
		udpConn, err = net.ListenUDP("udp4", &net.UDPAddr{
			IP:   addr.AsSlice(),
			Port: int(addrPort.Port()),
		})
	}
	if err != nil {
		return err
	}

	trans := &quic.Transport{Conn: udpConn}

	if runStun {
		// Run STUN server in a goroutine.
		stunCtx, stunCancel := context.WithCancel(s.ctx)
		defer stunCancel()

		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.logger.Error(
						"STUN server panicked",
						"service", "server.Server",
						"addr", address,
						"error", rec,
					)
				}
			}()

			stunErr := stun.RunServer(
				stunCtx,
				func(ctx context.Context, b []byte) (int, netip.AddrPort, error) {
					n, src, readErr := trans.ReadNonQUICPacket(ctx, b)
					if readErr != nil {
						return 0, netip.AddrPort{}, readErr
					}

					return n, netip.MustParseAddrPort(src.String()), nil
				},
				udpConn.WriteToUDPAddrPort,
			)
			if stunErr != nil {
				if protocol.IsErrorConnCloseOrCancel(stunErr) {
					return
				}

				s.logger.Error("STUN server RunServer function returned an error",
					"service", "server.Server",
					"addr", address,
					"error", stunErr,
				)
			}
		}()
	}

	listener, err := protocol.NewQuicProtoListenerFromTransport(trans, tlsCfg)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	return s.ListenWith(listener)
}
