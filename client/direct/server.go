package direct

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// handshakeTimeout is the timeout to wait for the handshake message from new direct connections.
const handshakeTimeout = 10 * time.Second

// Server is a direct connect server that accepts new direct connections from clients.
// It does not perform any authentication, it simply sends the connections along with
// their handshake messages to the appropriate Partition.
type Server struct {
	logger *slog.Logger

	ctx       context.Context
	ctxCancel context.CancelFunc

	m *Manager

	// The server's address and port.
	// Do not update.
	AddrPort netip.AddrPort

	listener protocol.ProtoListener
}

// NewServer creates a new direct connect server.
// It returns an error if a listener could not be created.
// Once created, it listens and handles incoming connections on its own.
func NewServer(
	logger *slog.Logger,
	ctx context.Context,
	m *Manager,
	addrPort netip.AddrPort,
	cert tls.Certificate,
) (*Server, error) {
	listener, err := protocol.NewQuicProtoListener(addrPort.String(), &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{protocol.DirectAlpnProtoName},
	})
	if err != nil {
		return nil, err
	}

	childCtx, ctxCancel := context.WithCancel(ctx)

	s := &Server{
		logger: logger,

		ctx:       childCtx,
		ctxCancel: ctxCancel,

		m: m,

		AddrPort: addrPort,

		listener: listener,
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("direct server run method panicked",
					"service", "direct.Server",
					"err", rec,
				)
			}
		}()
		defer func() {
			_ = s.Close()
		}()

		runErr := s.run()
		if runErr != nil {
			s.logger.Error("direct server run method exited with error",
				"service", "direct.Server",
				"err", runErr,
			)
		}
	}()

	return s, nil
}

// Close closes the server.
func (s *Server) Close() error {
	select {
	case <-s.ctx.Done():
		// Already closed, either by a previous call to Close or the Manager being closed.
		return nil
	default:
	}

	s.ctxCancel()

	// Remove server from server map.
	s.m.lockAndRemoveServer(s.AddrPort)

	return nil
}

// run runs the server accept loop.
// It exits with nil if the server was closed, or an error if there was an error accepting a connection.
func (s *Server) run() error {
	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if protocol.IsErrorConnCloseOrCancel(err) {
				return nil
			}

			return fmt.Errorf(`failed to accept direct connection: %w`, err)
		}

		go s.connHandler(conn)
	}
}

func (s *Server) connHandler(conn protocol.ProtoConn) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("connHandler panicked",
				"service", "direct.Server",
				"addr", conn.RemoteAddr().String(),
				"err", rec,
			)
		}
	}()

	isOk := false
	ctx, cancel := context.WithTimeout(s.ctx, handshakeTimeout)
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

		s.logger.Error("failed to wait for bidi from unauthenticated direct connection",
			"service", "direct.Server",
			"err", waitErr,
			"remote_addr", conn.RemoteAddr().String(),
		)
	}

	msg, err := protocol.ReadExpect[*pb.MsgDirectConnHandshake](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE)
	if err != nil {
		var streamErr *quic.StreamError
		if protocol.IsErrorConnCloseOrCancel(err) ||
			errors.As(err, &streamErr) {
			return
		}

		if unexpectedErr, ok := errors.AsType[protocol.UnexpectedMsgTypeError](err); ok {
			s.logger.Error("received unexpected message type during direct conn handshake",
				"service", "direct.Server",
				"expected_type", unexpectedErr.Expected.String(),
				"actual_type", unexpectedErr.Actual.String(),
				"remote_addr", conn.RemoteAddr().String(),
			)
			return
		}

		s.logger.Error("failed to read direct conn handshake message",
			"service", "direct.Server",
			"err", err,
			"remote_addr", conn.RemoteAddr().String(),
		)

		return
	}

	incoming := &IncomingDirectConn{
		conn:      conn,
		Handshake: msg.Payload,
		Bidi:      bidi,
	}

	// Determine partition based on method ID.
	part, has := s.m.getPartByMethodId(msg.Payload.MethodId)
	if !has {
		// No partition to handle connection.
		_ = incoming.InvalidToken()
		return
	}

	isOk = true

	// Send to partition.
	part.sendConn(incoming)
}
