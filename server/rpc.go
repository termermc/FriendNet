package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/common"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"friendnet.org/server/config"
)

// ErrRpcServerClosed is returned by methods of RpcServer if the server is closed.
var ErrRpcServerClosed = errors.New("RPC server is closed")

var errMissingBearerToken = connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
var errInvalidBearerToken = connect.NewError(connect.CodePermissionDenied, errors.New("invalid bearer token"))
var errIpNotAllowed = connect.NewError(connect.CodePermissionDenied, errors.New("IP not allowed"))

// InvalidRpcProtocolError is returned if an invalid RPC protocol version is specified.
type InvalidRpcProtocolError struct {
	// The invalid protocol
	Protocol string
}

func (e *InvalidRpcProtocolError) Error() string { return "invalid RPC protocol: " + e.Protocol }

// RpcServer implements an RPC server for a FriendNet server.
// It is a single instance that runs on a single interface.
type RpcServer struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	// The RPC server's address.
	// Do not update.
	Addr string

	server *Server

	httpServer   http.Server
	httpListener net.Listener

	checkIp    bool
	allowedIps map[netip.Addr]struct{}

	bearerToken string

	isAllMethodsAllowed bool
	allowedMethods      map[string]struct{}
}

type rpcServerInterceptor struct {
	s *RpcServer
}

var _ connect.Interceptor = rpcServerInterceptor{}

func (i rpcServerInterceptor) logic(peer connect.Peer, reqHeaders http.Header) error {
	// Check IP.
	if i.s.checkIp {
		host, _, _ := net.SplitHostPort(peer.Addr)

		{
			hostLen := len(host)
			if hostLen <= 2 {
				return errIpNotAllowed
			}

			// Remove brackets on IPv6.
			if host[0] == '[' && host[hostLen-1] == ']' {
				host = host[1 : hostLen-1]
			}
		}

		// Parse IP.
		peerIp, err := netip.ParseAddr(host)
		if err != nil {
			// Invalid IP string.
			return errIpNotAllowed
		}

		// Check if IP is allowed.
		_, has := i.s.allowedIps[peerIp]
		if !has {
			return errIpNotAllowed
		}
	}

	// Check authorization.
	if i.s.bearerToken != "" {
		authz := reqHeaders.Get("Authorization")
		if authz == "" {
			return errMissingBearerToken
		}

		token := strings.TrimPrefix(authz, "Bearer ")
		if token != i.s.bearerToken {
			return errInvalidBearerToken
		}
	}

	// We do not have access to the method name, so guarding allowed methods is done inside the service implementation.

	return nil
}

func (i rpcServerInterceptor) WrapUnary(fn connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.logic(req.Peer(), req.Header()); err != nil {
			return nil, err
		}

		return fn(ctx, req)
	}
}

func (i rpcServerInterceptor) WrapStreamingClient(fn connect.StreamingClientFunc) connect.StreamingClientFunc {
	// Not applicable.
	return fn
}

func (i rpcServerInterceptor) WrapStreamingHandler(fn connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.logic(conn.Peer(), conn.RequestHeader()); err != nil {
			return err
		}

		return fn(ctx, conn)
	}
}

func NewRpcServer(
	logger *slog.Logger,
	iface config.ServerRpcConfigInterface,
	server *Server,
) (*RpcServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	var isAllAllowed bool
	var allowedMethods map[string]struct{}
	if len(iface.AllowedMethods) == 1 && iface.AllowedMethods[0] == "*" {
		isAllAllowed = true
		allowedMethods = nil
	} else {
		isAllAllowed = false
		allowedMethods = make(map[string]struct{}, len(iface.AllowedMethods))
		for _, method := range iface.AllowedMethods {
			allowedMethods[method] = struct{}{}
		}
	}

	var checkIp bool
	var allowedIps map[netip.Addr]struct{}
	if len(iface.AllowedIps) > 0 {
		checkIp = true
		allowedIps = make(map[netip.Addr]struct{}, len(iface.AllowedIps))

		for _, ipStr := range iface.AllowedIps {
			ip, err := netip.ParseAddr(ipStr)
			if err != nil {
				cancel()
				return nil, fmt.Errorf(`invalid IP address %q in server RPC allowed IPs list: %w`, ipStr, err)
			}

			allowedIps[ip] = struct{}{}
		}
	} else {
		checkIp = false
		allowedIps = nil
	}

	s := &RpcServer{
		logger: logger,

		ctx:       ctx,
		ctxCancel: cancel,

		Addr:   iface.Address,
		server: server,

		checkIp:    checkIp,
		allowedIps: allowedIps,

		bearerToken: iface.BearerToken,

		isAllMethodsAllowed: isAllAllowed,
		allowedMethods:      make(map[string]struct{}),
	}

	// Figure out listener protocol.
	var listener net.Listener
	protoIdx := strings.Index(iface.Address, "://")
	if protoIdx == -1 {
		return nil, fmt.Errorf(`RPC server address %q is missing a protocol (should be something like "http://127.0.0.1:8080" or "unix:///tmp/server.sock")`, iface.Address)
	}
	proto := strings.ToLower(iface.Address[:protoIdx])
	protoAddr := iface.Address[protoIdx+3:]
	switch proto {
	case "http":
		// Create basic HTTP listener.
		var err error
		listener, err = net.Listen("tcp", protoAddr)
		if err != nil {
			return nil, fmt.Errorf(`failed to listen on TCP address %q: %w`, protoAddr, err)
		}
	case "unix":
		// First, resolve absolute path.
		unixPath, err := filepath.Abs(protoAddr)
		if err != nil {
			return nil, fmt.Errorf(`failed to resolve absolute path for UNIX socket path %q: %w`, protoAddr, err)
		}

		// If set, validate specified file permission.
		var permOctal os.FileMode
		if iface.FilePermission == "" {
			permOctal = 0600
		} else {
			octal, permErr := common.ParseGoOctalLiteral(iface.FilePermission)
			if permErr != nil {
				return nil, fmt.Errorf(`invalid file permission %q for UNIX socket path %q: %w`, iface.FilePermission, unixPath, permErr)
			}

			permOctal = os.FileMode(octal)
		}

		// Check if the containing directory exists.
		unixDir := filepath.Dir(unixPath)
		{
			info, statErr := os.Stat(unixDir)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					return nil, fmt.Errorf(`containing directory %q for UNIX path %q does not exist (server will not create it manually)`, unixDir, unixPath)
				}

				return nil, fmt.Errorf(`failed to stat containing directory %q for UNIX path %q: %w`, unixDir, unixPath, statErr)
			}

			if !info.IsDir() {
				return nil, fmt.Errorf(`containing directory %q for UNIX path %q exists, but is not a directory`, unixDir, unixPath)
			}
		}

		// Stat path to figure out what's there, if anything.
		{
			info, statErr := os.Lstat(unixPath)
			if statErr != nil {
				if !os.IsNotExist(statErr) {
					return nil, fmt.Errorf(`failed to stat UNIX socket path %q: %w`, unixPath, statErr)
				}
			} else if info.Mode().IsDir() {
				return nil, fmt.Errorf(`UNIX socket path %q points to a directory`, unixPath)
			} else if info.Mode()&os.ModeSocket == 0 {
				return nil, fmt.Errorf(`there is already a file at UNIX socket path %q, but it is not a UNIX socket`, unixPath)
			} else {
				// Socket already exists at path; delete it.
				delErr := os.Remove(unixPath)
				if delErr != nil {
					return nil, fmt.Errorf(`failed to delete existing UNIX socket at path %q: %w`, unixPath, delErr)
				}
			}
		}

		// Socket path is validated and any old socket there was removed.

		listener, err = net.Listen("unix", unixPath)
		if err != nil {
			return nil, fmt.Errorf(`failed to listen on UNIX socket path %q: %w`, unixPath, err)
		}
		err = os.Chmod(unixPath, permOctal)
		if err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf(`failed to set file permission %q for UNIX socket path %q: %w`, iface.FilePermission, unixPath, err)
		}

	default:
		return nil, fmt.Errorf(`unsupported protocol %q in server RPC address %q`, proto, iface.Address)
	}

	impl := &rpcServerImpl{
		s: s,
	}
	handlerPath, handler := serverrpcv1connect.NewServerRpcServiceHandler(impl,
		connect.WithInterceptors(rpcServerInterceptor{s: s}),
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("Hi, you've reached the RPC interface of a FriendNet server.\nYou can communicate with it using gRPC, gRPC-Web, and ConnectRPC.\nHave fun!\n"))
	})
	mux.Handle(handlerPath, handler)

	httpProtos := &http.Protocols{}
	httpProtos.SetHTTP1(true)
	httpProtos.SetHTTP2(true)
	httpProtos.SetUnencryptedHTTP2(true)

	s.httpServer = http.Server{
		Handler:   mux,
		Protocols: httpProtos,
	}
	s.httpListener = listener

	return s, nil
}

func (s *RpcServer) isMethodAllowed(method string) bool {
	if s.isAllMethodsAllowed {
		return true
	}

	_, ok := s.allowedMethods[method]
	return ok
}

// Serve starts the RPC server and runs until Close is called or an error occurs.
// The server closes after this returns, regardless of the return value.
// Returns ErrRpcServerClosed if the server is already closed.
func (s *RpcServer) Serve() error {
	s.mu.RLock()
	if s.isClosed {
		s.mu.RUnlock()
		return ErrRpcServerClosed
	}
	s.mu.RUnlock()

	defer func() {
		_ = s.Close()
	}()

	err := s.httpServer.Serve(s.httpListener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

// Close closes the RPC server and disconnects any currently connected clients of it.
// Subsequent calls are no-op.
func (s *RpcServer) Close() error {
	s.mu.Lock()
	if s.isClosed {
		s.mu.Unlock()
		return nil
	}
	s.isClosed = true
	s.mu.Unlock()

	shutdownCtx, shutdownCancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer shutdownCancel()

	_ = s.httpServer.Shutdown(shutdownCtx)
	_ = s.httpListener.Close()

	return nil
}
