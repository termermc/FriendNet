package common

import (
	"context"
	"errors"
	"fmt"
	"io"
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
)

// RpcServerConfig is the configuration for a single RPC server instance.
// Can be JSON (de)serialized.
type RpcServerConfig struct {
	// The address to bind to.
	// Must be in the format "PROTOCOL://HOST:PORT" (or without port for unix).
	//
	// Supported protocols:
	//  - http
	//  - unix
	//
	// Examples:
	//  - "http://127.0.0.1:8080"
	//  - "unix:///var/run/friendnet-server.sock" (/var/run/friendnet-server.sock, absolute path)
	//  - "unix://friendnet-server.sock" (friendnet-server.sock, relative path)
	//
	// The unix protocol will create a file with 0600 permission by default.
	// To set the permission, set the "file_permission" field.
	// Windows support for the unix protocol is not supported but may work.
	Address string `json:"address"`

	// The RPC methods that are allowed to be called on this interface.
	// Consult the rpc.proto file to see a full list of methods.
	//
	// An empty or null list will prevent any methods from being called.
	//
	// To explicitly allow all methods, include a single string with the value "*".
	//
	// Example: ["GetRooms", "GetRoomInfo", "GetOnlineUsers", "GetOnlineUserInfo"]
	AllowedMethods []string `json:"allowed_methods"`

	// If not null or empty, only the specified IP addresses will be allowed to connect.
	// Has no effect if the address protocol is unix.
	AllowedIps []string `json:"allowed_ips,omitempty"`

	// The file mode to use for the unix socket file.
	// Must be in format "0600", starting with a zero followed by three octal digits.
	// Has no effect if the address protocol is not unix.
	FilePermission string `json:"file_permission,omitempty"`

	// If not null or empty, the following HTTP bearer token will be required to access the RPC interface.
	// For example, if set to "abc123", the following HTTP header must be set: "Authorization: Bearer abc123".
	BearerToken string `json:"bearer_token,omitempty"`

	// If true, sets necessary CORS headers to allow cross-origin requests.
	// You do not need this unless the RPC interface is accessed by web browsers.
	CorsAllowAllOrigins bool `json:"cors_allow_all_origins"`
}

// RpcHandlerConstructor is a constructor for creating an RPC handler.
// It returns the path to mount it on and the handler itself.
type RpcHandlerConstructor[T any] = func(impl T, options ...connect.HandlerOption) (string, http.Handler)

// ErrRpcServerClosed is returned by methods of RpcServer if the server is closed.
var ErrRpcServerClosed = errors.New("RPC server is closed")

var errMissingBearerToken = connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
var errInvalidBearerToken = connect.NewError(connect.CodePermissionDenied, errors.New("invalid bearer token"))
var errIpNotAllowed = connect.NewError(connect.CodePermissionDenied, errors.New("IP not allowed"))
var errMethodNotAllowed = connect.NewError(connect.CodePermissionDenied, errors.New("method not allowed"))

// InvalidRpcProtocolError is returned if an invalid RPC protocol version is specified.
type InvalidRpcProtocolError struct {
	// The invalid protocol.
	Protocol string
}

func (e *InvalidRpcProtocolError) Error() string { return "invalid RPC protocol: " + e.Protocol }

// TODO Move this to common, make it a generic struct to support multiple implementations but keep other logic

// RpcServer implements an RPC server for a FriendNet server.
// It is a single instance that runs on a single interface.
type RpcServer[T io.Closer] struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	// The RPC server's address.
	// Do not update.
	Addr string

	impl T

	httpServer   http.Server
	httpListener net.Listener

	corsAllowAllOrigins bool
}

type rpcServerInterceptor struct {
	checkIp    bool
	allowedIps map[netip.Addr]struct{}

	bearerToken string

	isAllMethodsAllowed bool
	// Keys are lowercase.
	allowedMethods map[string]struct{}
}

var _ connect.Interceptor = rpcServerInterceptor{}

func (i rpcServerInterceptor) logic(peer connect.Peer, spec connect.Spec, reqHeaders http.Header) error {
	// Check IP.
	if i.checkIp {
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
		_, has := i.allowedIps[peerIp]
		if !has {
			return errIpNotAllowed
		}
	}

	// Check authorization.
	if i.bearerToken != "" {
		authz := reqHeaders.Get("Authorization")
		if authz == "" {
			return errMissingBearerToken
		}

		token := strings.TrimPrefix(authz, "Bearer ")
		if token != i.bearerToken {
			return errInvalidBearerToken
		}
	}

	// Check method.
	if !i.isAllMethodsAllowed {
		path := strings.TrimSuffix(spec.Procedure, "/")
		var methodLower string
		{
			slashIdx := strings.LastIndex(path, "/")
			if slashIdx == -1 {
				methodLower = strings.ToLower(path)
			} else {
				methodLower = strings.ToLower(path[slashIdx+1:])
			}
		}
		if _, has := i.allowedMethods[methodLower]; !has {
			return errMethodNotAllowed
		}
	}

	return nil
}

func (i rpcServerInterceptor) WrapUnary(fn connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.logic(req.Peer(), req.Spec(), req.Header()); err != nil {
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
		if err := i.logic(conn.Peer(), conn.Spec(), conn.RequestHeader()); err != nil {
			return err
		}

		return fn(ctx, conn)
	}
}

func NewRpcServer[T io.Closer](
	logger *slog.Logger,
	cfg RpcServerConfig,
	impl T,
	constructor RpcHandlerConstructor[T],
) (*RpcServer[T], error) {
	ctx, cancel := context.WithCancel(context.Background())

	var isAllAllowed bool
	var allowedMethods map[string]struct{}
	if len(cfg.AllowedMethods) == 1 && cfg.AllowedMethods[0] == "*" {
		isAllAllowed = true
		allowedMethods = nil
	} else {
		isAllAllowed = false
		allowedMethods = make(map[string]struct{}, len(cfg.AllowedMethods))
		for _, method := range cfg.AllowedMethods {
			allowedMethods[strings.ToLower(method)] = struct{}{}
		}
	}

	var checkIp bool
	var allowedIps map[netip.Addr]struct{}
	if len(cfg.AllowedIps) > 0 {
		checkIp = true
		allowedIps = make(map[netip.Addr]struct{}, len(cfg.AllowedIps))

		for _, ipStr := range cfg.AllowedIps {
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

	s := &RpcServer[T]{
		logger: logger,

		ctx:       ctx,
		ctxCancel: cancel,

		Addr: cfg.Address,

		impl: impl,

		corsAllowAllOrigins: cfg.CorsAllowAllOrigins,
	}

	// Figure out listener protocol.
	var listener net.Listener
	protoIdx := strings.Index(cfg.Address, "://")
	if protoIdx == -1 {
		return nil, fmt.Errorf(`RPC server address %q is missing a protocol (should be something like "http://127.0.0.1:8080" or "unix:///tmp/server.sock")`, cfg.Address)
	}
	proto := strings.ToLower(cfg.Address[:protoIdx])
	protoAddr := cfg.Address[protoIdx+3:]
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
		if cfg.FilePermission == "" {
			permOctal = 0600
		} else {
			octal, permErr := ParseGoOctalLiteral(cfg.FilePermission)
			if permErr != nil {
				return nil, fmt.Errorf(`invalid file permission %q for UNIX socket path %q: %w`, cfg.FilePermission, unixPath, permErr)
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
			return nil, fmt.Errorf(`failed to set file permission %q for UNIX socket path %q: %w`, cfg.FilePermission, unixPath, err)
		}

	default:
		return nil, fmt.Errorf(`unsupported protocol %q in server RPC address %q`, proto, cfg.Address)
	}

	handlerPath, handler := constructor(impl,
		connect.WithInterceptors(rpcServerInterceptor{
			checkIp:    checkIp,
			allowedIps: allowedIps,

			bearerToken: cfg.BearerToken,

			isAllMethodsAllowed: isAllAllowed,
			allowedMethods:      make(map[string]struct{}),
		}),
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("Hi, you've reached the RPC interface.\nYou can communicate with it using gRPC, gRPC-Web, and ConnectRPC.\nHave fun!\n"))
	})
	mux.HandleFunc(handlerPath, func(w http.ResponseWriter, r *http.Request) {
		if s.corsAllowAllOrigins {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler.ServeHTTP(w, r)
	})

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

// Serve starts the RPC server and runs until Close is called or an error occurs.
// The server closes after this returns, regardless of the return value.
// Returns ErrRpcServerClosed if the server is already closed.
func (s *RpcServer[T]) Serve() error {
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
func (s *RpcServer[T]) Close() error {
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

	_ = s.impl.Close()

	return nil
}
