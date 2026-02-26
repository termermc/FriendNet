package protocol

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"friendnet.org/client/cert"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// ErrUnknownMethodType is returned when trying to use an unknown connection method type.
var ErrUnknownMethodType = errors.New("unknown direct connection method type")

// IsMethodTypeKnown returns whether the specified connection method type is known to the current protocol version.
// Useful when paired with ErrUnknownMethodType.
func IsMethodTypeKnown(typ pb.ConnMethodType) bool {
	return typ == pb.ConnMethodType_CONN_METHOD_TYPE_IP ||
		typ == pb.ConnMethodType_CONN_METHOD_TYPE_YGGDRASIL
}

// ValidateMethodAddress attempts to validate the address for the specified method type.
//
// It does not attempt to validate the address if it does not know about the method type.
// Instead, it will just return nil.
// This behavior is to allow for clients on newer protocol versions to advertise new
// method types that are unknown to the server's protocol.
func ValidateMethodAddress(typ pb.ConnMethodType, address string) error {
	switch typ {
	case pb.ConnMethodType_CONN_METHOD_TYPE_IP:
		_, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf(`address %q is in incorrect format for method %s: %w`, address, typ.String(), err)
		}
		return nil
	case pb.ConnMethodType_CONN_METHOD_TYPE_YGGDRASIL:
		addrPort, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf(`address %q is in incorrect format for method %s: %w`, address, typ.String(), err)
		}
		if !addrPort.Addr().Is6() {
			return fmt.Errorf(`only IPv6 addresses are valid Yggdrasil addresses`)
		}
		return nil
	default:
		// We do not know about this method type, so we cannot validate it.
		return nil
	}
}

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
// It returns the pb.ConnResult that corresponds with the error returned, or CONN_RESULT_OK if no error.
//
// The address format is defined by the method type.
// Support for the method type is not checked; it is the caller's responsibility to check for support beforehand.
//
// If the method type is unknown, it will return ErrUnknownMethodType.
// If the server returns OK, it will return a ProtoConn.
// If the server returns anything else, it will return a DirectConnHandshakeError.
//
// This function does not apply its own timeout; that should be done with the context passed in.
func CreateDirectConnection(
	ctx context.Context,
	methodType pb.ConnMethodType,
	address string,
	handshake *pb.MsgDirectConnHandshake,
) (conn ProtoConn, result pb.ConnResult, err error) {
	conn, err = func() (ProtoConn, error) {
		if !IsMethodTypeKnown(methodType) {
			return nil, ErrUnknownMethodType
		}

		if err = ValidateMethodAddress(methodType, address); err != nil {
			return nil, err
		}

		// Currently, all known methods connect using IP:PORT.
		// We can be sure that splitting works because we already checked the format.
		hostname, _, _ := net.SplitHostPort(address)
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

		var qConn *quic.Conn
		qConn, err = quic.DialAddr(ctx, address, tlsCfg, &quic.Config{
			KeepAlivePeriod: DefaultKeepAlivePeriod,
		})
		if err != nil {
			return nil, fmt.Errorf(`failed to dial QUIC %q for direct connection: %w`, address, err)
		}

		conn = ToProtoConn(qConn)

		isOk := false
		const timedOutMsg = "test timed out"
		const canceledMsg = "test canceled"
		go func(c ProtoConn) {
			<-ctx.Done()
			if isOk {
				return
			}

			ctxErr := ctx.Err()
			if errors.Is(ctxErr, context.Canceled) {
				_ = c.CloseWithReason(canceledMsg)
				return
			}
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				_ = c.CloseWithReason(timedOutMsg)
				return
			}

			_ = c.CloseWithReason("")
		}(conn)

		// Send handshake.
		msg, hsErr := SendAndReceiveExpect[*pb.MsgDirectConnHandshakeResult](
			conn,
			pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE,
			handshake,
			pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT,
		)
		if hsErr != nil {
			if appErr, ok := errors.AsType[*quic.ApplicationError](hsErr); ok {
				if appErr.ErrorMessage == timedOutMsg || appErr.ErrorMessage == canceledMsg {
					return nil, context.DeadlineExceeded
				}
			}
			return nil, fmt.Errorf(`handshake failed when direct connecting to %q: %w`, address, hsErr)
		}

		if msg.Payload.Result == pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_OK {
			// The connection is authenticated and ready to be used.
			isOk = true
			return conn, nil
		}

		return nil, DirectConnHandshakeError{
			Result: msg.Payload.Result,
		}
	}()
	if err != nil {
		if errors.Is(err, ErrUnknownMethodType) {
			result = pb.ConnResult_CONN_RESULT_METHOD_NOT_SUPPORTED
			return
		}

		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled) {
			result = pb.ConnResult_CONN_RESULT_TIMED_OUT
			return
		}
		if _, ok := errors.AsType[*quic.IdleTimeoutError](err); ok {
			result = pb.ConnResult_CONN_RESULT_TIMED_OUT
			return
		}

		if hsErr, ok := errors.AsType[DirectConnHandshakeError](err); ok {
			if hsErr.IsKThxBye() {
				result = pb.ConnResult_CONN_RESULT_OK
				return
			}

			result = pb.ConnResult_CONN_RESULT_HANDSHAKE_FAILED
			return
		}

		if _, ok := errors.AsType[*quic.StreamError](err); ok {
			result = pb.ConnResult_CONN_RESULT_CONN_REFUSED
			return
		}
		if _, ok := errors.AsType[*quic.ApplicationError](err); ok {
			result = pb.ConnResult_CONN_RESULT_CONN_REFUSED
			return
		}

		result = pb.ConnResult_CONN_RESULT_INTERNAL_ERROR
		return
	}

	result = pb.ConnResult_CONN_RESULT_OK
	return
}
