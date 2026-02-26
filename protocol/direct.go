package protocol

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"

	"friendnet.org/client/cert"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// DirectConnHandshakeError is returned when a direct connection handshake's result is not DIRECT_CONN_HANDSHAKE_RESULT_OK.
type DirectConnHandshakeError struct {
	// The result returned by the direct server.
	Result pb.DirectConnHandshakeResult
}

var _ error = DirectConnHandshakeError{}

func (e DirectConnHandshakeError) Error() string {
	const prefix = "direct server returned handshake error: "
	if e.IsTokenInvalid() {
		return prefix + "token is invalid"
	}
	if e.IsInternalError() {
		return prefix + "internal error"
	}
	if e.IsKThxBye() {
		return prefix + "accepted but disconnected (kthxbye)"
	}

	return prefix + e.Result.String()
}

func (e DirectConnHandshakeError) IsTokenInvalid() bool {
	return e.Result == pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_TOKEN_INVALID
}
func (e DirectConnHandshakeError) IsInternalError() bool {
	return e.Result == pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_INTERNAL_ERROR
}
func (e DirectConnHandshakeError) IsKThxBye() bool {
	return e.Result == pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_KTHXBYE
}

// CreateDirectConnection attempts to make a direct connection to the server at addr with the provided handshake.
// The address should be in the format IP:PORT.
//
// If the server returns OK, it will return a ProtoConn.
// If the server returns anything else, it will return a DirectConnHandshakeError.
//
// This function does not apply its own timeout; that should be done with the context passed in.
func CreateDirectConnection(ctx context.Context, address string, handshake *pb.MsgDirectConnHandshake) (ProtoConn, error) {
	hostname, _, parseErr := net.SplitHostPort(address)
	if parseErr != nil {
		return nil, fmt.Errorf(`failed to parse address %q in CreateDirectConnection: %w`, address, parseErr)
	}
	hostname = cert.NormalizeHostname(hostname)

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{DirectAlpnProtoName},
		ServerName:         hostname,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return ErrNoServerCerts
			}

			// Allow any certificate.
			// Direct servers all use self-signed certs.
			// Verification is done via tokens issued by the central server.
			return nil
		},
	}

	qConn, err := quic.DialAddr(ctx, address, tlsCfg, &quic.Config{
		KeepAlivePeriod: DefaultKeepAlivePeriod,
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to dial QUIC %q for direct connection: %w`, address, err)
	}

	conn := ToProtoConn(qConn)

	isOk := false
	go func() {
		if isOk {
			return
		}

		<-ctx.Done()
		ctxErr := ctx.Err()
		if errors.Is(ctxErr, context.Canceled) {
			_ = conn.CloseWithReason("cancelled")
			return
		}
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			_ = conn.CloseWithReason("timed out")
			return
		}

		_ = conn.CloseWithReason("")
	}()

	// Send handshake.
	msg, err := SendAndReceiveExpect[*pb.MsgDirectConnHandshakeResult](
		conn,
		pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE,
		handshake,
		pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT,
	)
	if err != nil {
		return nil, fmt.Errorf(`handshake failed when direct connecting to %q: %w`, address, err)
	}

	if msg.Payload.Result == pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_OK {
		isOk = true
		return conn, nil
	}

	return nil, DirectConnHandshakeError{
		Result: msg.Payload.Result,
	}
}
