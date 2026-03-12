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
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrWebServerClosed is returned when calling methods on a closed WebServer.
var ErrWebServerClosed = errors.New("WebServer closed closed")

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

type serverInst struct {
	Addr     string
	Server   *http.Server
	ServeMux *http.ServeMux
	Listener net.Listener
}

// Option is a WebServer option function.
type Option func(ws *WebServer)

// WithHttpsSupport enables HTTPS support using the specified certificate.
func WithHttpsSupport(cert tls.Certificate) Option {
	return func(ws *WebServer) {
		ws.httpsCertOrNil = &cert
	}
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
	servers map[string]*serverInst
}

// NewWebServer creates a new WebServer.
func NewWebServer(
	logger *slog.Logger,
	opts ...Option,
) *WebServer {
	ws := &WebServer{
		logger: logger,

		httpsCertOrNil: nil,

		servers: make(map[string]*serverInst),
	}

	for _, opt := range opts {
		opt(ws)
	}

	return ws
}

func (ws *WebServer) Close() error {
	ws.mu.Lock()
	if ws.isClosed {
		ws.mu.Unlock()
		return nil
	}
	ws.isClosed = true
	servers := make([]*serverInst, 0, len(ws.servers))
	for _, server := range ws.servers {
		servers = append(servers, server)
	}
	ws.servers = nil
	ws.mu.Unlock()

	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Go(func() {
			_ = server.Server.Close()
		})
	}
	wg.Wait()
	return nil
}

type mountOptions struct {
	unixFilePerm os.FileMode
}

// MountOption is a WebServer.Mount option function.
type MountOption func(opts *mountOptions)

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
//
//goland:noinspection GoRedundantElseInIf
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

	var server *serverInst

	ws.mu.Lock()
	if ws.isClosed {
		ws.mu.Unlock()
		return ErrWebServerClosed
	}
	server, _ = ws.servers[normalAddr]
	ws.mu.Unlock()

	if server == nil {
		// A new HTTP server must be created.

		var protos http.Protocols
		protos.SetHTTP2(true)
		protos.SetHTTP1(true)

		var listener net.Listener
		var err error
		switch proto {
		case "https":
			listener, err = tls.Listen("tcp", addr, &tls.Config{
				Certificates: []tls.Certificate{*ws.httpsCertOrNil},
				NextProtos:   []string{"h2"},
			})
			if err != nil {
				return fmt.Errorf(`failed to listen on TLS address %q: %w`, addr, err)
			}
		case "http":
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf(`failed to listen on TCP address %q: %w`, addr, err)
			}
		case "unix":
			// If set, validate specified file permission.
			const permOctal = os.FileMode(0600)

			// Check if the containing directory exists.
			unixDir := filepath.Dir(addr)
			{
				info, statErr := os.Stat(unixDir)
				if statErr != nil {
					if os.IsNotExist(statErr) {
						return fmt.Errorf(`containing directory %q for UNIX path %q does not exist (server will not create it manually)`, unixDir, addr)
					}

					return fmt.Errorf(`failed to stat containing directory %q for UNIX path %q: %w`, unixDir, addr, statErr)
				}

				if !info.IsDir() {
					return fmt.Errorf(`containing directory %q for UNIX path %q exists, but is not a directory`, unixDir, addr)
				}
			}

			// Stat path to figure out what's there, if anything.
			{
				info, statErr := os.Lstat(addr)
				if statErr != nil {
					if !os.IsNotExist(statErr) {
						return fmt.Errorf(`failed to stat UNIX socket path %q: %w`, addr, statErr)
					}
				} else if info.Mode().IsDir() {
					return fmt.Errorf(`UNIX socket path %q points to a directory`, addr)
				} else if info.Mode()&os.ModeSocket == 0 {
					return fmt.Errorf(`there is already a file at UNIX socket path %q, but it is not a UNIX socket`, addr)
				} else {
					// Socket already exists at path; delete it.
					delErr := os.Remove(addr)
					if delErr != nil {
						return fmt.Errorf(`failed to delete existing UNIX socket at path %q: %w`, addr, delErr)
					}
				}
			}

			// Socket path is validated and any old socket there was removed.

			listener, err = net.Listen("unix", addr)
			if err != nil {
				return fmt.Errorf(`failed to listen on UNIX socket path %q: %w`, addr, err)
			}
			err = os.Chmod(addr, permOctal)
			if err != nil {
				_ = listener.Close()
				return fmt.Errorf(`failed to set file permission %q for UNIX socket path %q: %w`, permOctal, addr, err)
			}
		default:
			panic(fmt.Errorf("BUG: unsupported protocol %q not caught early", proto))
		}

		mux := http.NewServeMux()

		httpServer := &http.Server{
			Protocols: &protos,
			Handler: recoverWrapper{
				ws:      ws,
				handler: mux,
			},
		}

		server = &serverInst{
			Addr:     normalAddr,
			Server:   httpServer,
			ServeMux: mux,
			Listener: listener,
		}
		ws.mu.Lock()
		ws.servers[normalAddr] = server
		ws.mu.Unlock()
	}

	server.ServeMux.Handle(path, handler)

	return nil
}

// Serve starts all HTTP servers.
// It returns when any of the servers returned an error, returning that same error.
// It never returns a non-nil error.
// If the servers were closed, returns http.ErrServerClosed.
// The WebServer instance is closed when this method returns.
func (ws *WebServer) Serve() error {
	ws.mu.Lock()
	if ws.isClosed {
		ws.mu.Unlock()
		return ErrWebServerClosed
	}
	servers := make([]*serverInst, 0, len(ws.servers))
	for _, server := range ws.servers {
		servers = append(servers, server)
	}
	ws.mu.Unlock()

	errChan := make(chan error, 1)

	for _, server := range servers {
		go func() {
			if serveErr := server.Server.Serve(server.Listener); serveErr != nil {
				errChan <- serveErr
			}
		}()
	}

	defer func() {
		_ = ws.Close()
	}()
	return <-errChan
}
