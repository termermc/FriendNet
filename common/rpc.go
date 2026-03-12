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
	"runtime/debug"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"friendnet.org/common/webserver"
)

// RpcServerConfig is the configuration for a single RPC server instance.
// Can be JSON (de)serialized.
type RpcServerConfig struct {
	// The address to bind to.
	// Must be in the format "PROTOCOL://HOST:PORT" (or without port for unix).
	//
	// Supported protocols:
	//  - http
	//  - https
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

// RpcServer implements an RPC server for a FriendNet server.
// It does not listen itself; the underlying listener is managed by the webserver.WebServer.
type RpcServer[T io.Closer] struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	webServer *webserver.WebServer

	// The RPC server's address.
	// Do not update.
	Addr string

	impl T

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
	webServer *webserver.WebServer,
	cfg RpcServerConfig,
	impl T,
	constructor RpcHandlerConstructor[T],
) (*RpcServer[T], error) {
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

		webServer: webServer,

		Addr: cfg.Address,

		impl: impl,

		corsAllowAllOrigins: cfg.CorsAllowAllOrigins,
	}

	handlerPath, handler := constructor(impl,
		connect.WithInterceptors(rpcServerInterceptor{
			checkIp:    checkIp,
			allowedIps: allowedIps,

			bearerToken: cfg.BearerToken,

			isAllMethodsAllowed: isAllAllowed,
			allowedMethods:      allowedMethods,
		}),
	)

	err := webServer.Mount(
		cfg.Address,
		handlerPath,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic in RCP handler",
						"service", "common.RpcServer",
						"err", rec,
						"stack", string(debug.Stack()),
					)
				}
			}()

			if s.corsAllowAllOrigins {
				origin := r.Header.Get("Origin")
				if origin == "" {
					origin = "*"
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Add("Access-Control-Allow-Headers", "*")
				w.Header().Add("Access-Control-Allow-Headers", "Authorization, Content-Type, connect-protocol-version")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			handler.ServeHTTP(w, r)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf(`failed to mount RPC handler on %q path %q: %w`, cfg.Address, handlerPath, err)
	}

	return s, nil
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

	_ = s.impl.Close()

	return nil
}
