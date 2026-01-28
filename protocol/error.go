package protocol

import (
	"errors"
	"fmt"

	pb "friendnet.org/protocol/pb/v1"
)

var (
	ErrCertStoreRequired       = errors.New("cert store is required")
	ErrProtocolVersionRequired = errors.New("protocol version is required")
	ErrNoServerCerts           = errors.New("no server certificates presented")
	ErrServerCertNotValidNow   = errors.New("server certificate is not valid at the current time")
)

// UnexpectedMsgTypeError is an error returned when expecting to receive a certain message type, but got another.
type UnexpectedMsgTypeError struct {
	// The expected message type.
	Expected pb.MsgType

	// The actual message type received.
	Actual pb.MsgType
}

func (e UnexpectedMsgTypeError) Error() string {
	return fmt.Sprintf("expected message type %s but got type %s", e.Expected.String(), e.Actual.String())
}

func NewUnexpectedMsgTypeError(expected pb.MsgType, actual pb.MsgType) UnexpectedMsgTypeError {
	return UnexpectedMsgTypeError{
		Expected: expected,
		Actual:   actual,
	}
}

// ProtoMsgError is an error returned when an error message is read from the protocol.
type ProtoMsgError struct {
	Msg *pb.MsgError
}

func (e ProtoMsgError) Error() string {
	var msg string
	if e.Msg.Message != nil {
		msg = ": " + *e.Msg.Message
	}

	return fmt.Sprintf(`received error message from protocol: %s%s`,
		e.Msg.Type.String(),
		msg,
	)
}

func NewProtoMsgError(msg *pb.MsgError) ProtoMsgError {
	return ProtoMsgError{
		Msg: msg,
	}
}

// CertMismatchError is returned when the server certificate changes for a host.
type CertMismatchError struct {
	Host string
}

func (e CertMismatchError) Error() string {
	return fmt.Sprintf("server certificate mismatch for %q", e.Host)
}

// VersionRejectedError is returned when the server rejects the client's protocol version.
type VersionRejectedError struct {
	Reason  pb.VersionRejectionReason
	Message string
}

func (e VersionRejectedError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("protocol version rejected: %s", e.Reason.String())
	}
	return fmt.Sprintf("protocol version rejected: %s: %s", e.Reason.String(), e.Message)
}

// AuthRejectedError is returned when the server rejects authentication.
type AuthRejectedError struct {
	Reason  pb.AuthRejectionReason
	Message string
}

func (e AuthRejectedError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("authentication rejected: %s", e.Reason.String())
	}
	return fmt.Sprintf("authentication rejected: %s: %s", e.Reason.String(), e.Message)
}
