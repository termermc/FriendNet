package webserver

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
)

type recoverWrapper struct {
	ws      *WebServer
	handler http.Handler
}

var _ http.Handler = recoverWrapper{}

func (rw recoverWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			rw.ws.logger.Error("panic in HTTP handler",
				"service", "webserver.WebServer",
				"url", r.URL.String(),
				"err", rec,
			)
		}
	}()

	rw.handler.ServeHTTP(w, r)
}

type serverAndRouter struct {
	*http.Server
	*http.ServeMux
}

// WebServer is an abstraction over HTTP servers.
// It allows mounting a handler on an address, like "https://127.0.0.1:20040/rpc".
// Multiple mounts with the same protocol and address will be mounted on the same underlying HTTP server.
// Closing WebServer closes all underlying HTTP servers.
type WebServer struct {
	mu       sync.RWMutex
	isClosed bool

	logger *slog.Logger

	httpsCertOrNil *tls.Certificate

	// Key: protocol + address (no path)
	// Example: https://127.0.0.1:20040
	// Example: unix:///tmp/friendnet.sock (UNIX must be absolute)
	servers map[string]*serverAndRouter
}

// NewWebServer creates a new WebServer.
// If the HTTPS cert is nil, the server will not be HTTPS.
func NewWebServer(
	logger *slog.Logger,

	httpsCertOrNil *tls.Certificate,
) *WebServer {
	return &WebServer{
		logger: logger,

		httpsCertOrNil: httpsCertOrNil,

		servers: make(map[string]*serverAndRouter),
	}
}

func (ws *WebServer) Close() error {
	ws.mu.Lock()
	if ws.isClosed {
		ws.mu.Unlock()
		return nil
	}
	ws.isClosed = true
	servers := make([]*serverAndRouter, 0, len(ws.servers))
	for _, server := range ws.servers {
		servers = append(servers, server)
	}
	ws.servers = nil
	ws.mu.Unlock()

	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Go(func() {
			_ = server.Close()
		})
	}
	wg.Wait()
	return nil
}

// Mount mounts a handler on an address and path.
// If there is no HTTP server running on the address, one will be created.
//
// Supported address formats:
//   - http://IP:PORT
//   - https://IP:PORT
//   - unix:///ABSOLUTE
//   - unix://RELATIVE
//
// Examples:
//   - http://127.0.0.1:20040/rpc
//   - https://[::1]:20040/rpc
//   - unix:///tmp/friendnet.sock
//   - unix://friendnet.sock
func (ws *WebServer) Mount(address string, path string, handler http.Handler) error {
	// Parse URL into protocol and address.
	var proto string
	var addr string
	var normalAddr string
	{
		u, err := url.Parse(address)
		if err != nil {
			return fmt.Errorf(`invalid address (must include a protocol and address): %w`, err)
		}

		proto = u.Scheme

		switch u.Scheme {
		case "https":
			if ws.httpsCertOrNil == nil {
				return fmt.Errorf(`the WebServer instance was not created with an HTTPS certificate, so HTTPS is not available`)
			}
			fallthrough
		case "http":
			addr = u.Host

			// Is there a path?
			if u.Path != "" {
				return fmt.Errorf(`address must not include a path, got %q`, addr)
			}

			// Is it an IP:PORT?
			_, err = netip.ParseAddrPort(addr)
			if err != nil {
				return fmt.Errorf(`HTTP/HTTPS addresses must use an IP:PORT hostname, got %q`, addr)
			}
		case "unix":
			addr = u.Host + u.Path
			if !strings.HasPrefix(addr, "/") {
				addr, err = filepath.Abs(addr)
				if err != nil {
					return fmt.Errorf(`invalid UNIX socket path %q: %w`, addr, err)
				}
			}
		default:
			return fmt.Errorf(`unsupported protocol %q`, proto)
		}

		normalAddr = proto + "://" + addr
	}

	var server *serverAndRouter

	ws.mu.Lock()
	if ws.isClosed {
		ws.mu.Unlock()
		return nil
	}
	server, _ = ws.servers[normalAddr]
	ws.mu.Unlock()

	if server == nil {
		// A new HTTP server must be created.

		var protos http.Protocols
		protos.SetHTTP2(true)
		protos.SetHTTP1(true)
		protos.SetUnencryptedHTTP2(true)

		var tlsCfg *tls.Config
		var listener net.Listener
		var err error
		switch proto {
		case "https":
			tlsCfg = &tls.Config{
				Certificates: []tls.Certificate{*ws.httpsCertOrNil},
			}
			fallthrough
		case "http":
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf(`failed to listen on TCP address %q: %w`, addr, err)
			}
		case "unix":
			listener, err = net.Listen("unix", addr)
			if err != nil {
				return fmt.Errorf(`failed to listen on UNIX socket path %q: %w`, addr, err)
			}
		default:
			panic(fmt.Errorf("BUG: unsupported protocol %q not caught early", proto))
		}

		mux := http.NewServeMux()

		httpServer := &http.Server{
			Protocols: &protos,
			TLSConfig: tlsCfg,
			Handler: recoverWrapper{
				ws:      ws,
				handler: mux,
			},
		}

		server = &serverAndRouter{
			Server:   httpServer,
			ServeMux: mux,
		}

		go func() {
			if serveErr := server.Server.Serve(listener); serveErr != nil {
				if errors.Is(serveErr, http.ErrServerClosed) {
					return
				}

				ws.logger.Error("HTTP server ended with error",
					"service", "webserver.WebServer",
					"addr", addr,
					"err", serveErr,
				)
			}
		}()
	}

	server.Handle(path, handler)

	return nil
}
