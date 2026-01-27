package protocol

import (
	"fmt"

	pb "friendnet.org/protocol/pb/v1"
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
