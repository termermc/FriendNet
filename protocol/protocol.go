package protocol

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"reflect"
	"time"

	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

// TODO Implement timeouts for all reads.

// ProxyPeerUnreachableStreamErrorCode is the code inside a quic.StreamError returned to a proxy initiator when the destination peer is unreachable.
const ProxyPeerUnreachableStreamErrorCode quic.StreamErrorCode = 101

// ErrPeerUnreachable is returned when a peer is unreachable.
var ErrPeerUnreachable = errors.New("peer unreachable")

const msgHeaderSize = 8

// CurrentProtocolVersion is the current protocol version used by the client and server modules in this codebase.
var CurrentProtocolVersion = &pb.ProtoVersion{
	Major: 0,
	Minor: 0,
	Patch: 0,
}

// DefaultKeepAlivePeriod is the default keepalive period for QUIC connections.
const DefaultKeepAlivePeriod = 10 * time.Second

// UntypedProtoMsg is a protocol message with an unknown payload type.
// It can be converted to a TypedProtoMsg with ToTyped.
// See documentation on ToTyped for details.
type UntypedProtoMsg struct {
	Type    pb.MsgType
	Payload proto.Message
}

// TypedProtoMsg is a protocol message with a known payload type.
type TypedProtoMsg[T proto.Message] struct {
	Type    pb.MsgType
	Payload T
}

// NewTypedProtoMsg creates a new TypedProtoMsg with the provided MsgType and payload.
func NewTypedProtoMsg[T proto.Message](typ pb.MsgType, payload T) *TypedProtoMsg[T] {
	return &TypedProtoMsg[T]{
		Type:    typ,
		Payload: payload,
	}
}

// ToTyped casts an UntypedProtoMsg to a TypedProtoMsg of the specified type.
// Panics if the type does not match the message's type.
//
// Assuming you got the UntypedProtoMsg from a ProtoBidi Read method, this method is safe to use as long as you checked the MsgType beforehand.
// The Read methods deserialize the payload to the correct underlying type, so it is impossible for the MsgType to mismatch.
// Still, use this function with caution.
func ToTyped[T proto.Message](msg *UntypedProtoMsg) *TypedProtoMsg[T] {
	casted, ok := msg.Payload.(T)
	if !ok {
		wantType := reflect.TypeFor[T]()
		gotType := reflect.TypeOf(msg)

		panic(fmt.Sprintf(`tried to cast UntypedProtoMsg with type enum %s to %s, but it was actually %s`,
			msg.Type.String(),
			wantType.String(),
			gotType.String(),
		))
	}

	return NewTypedProtoMsg(msg.Type, casted)
}

// ProtoConn is a protocol connection.
// You can send and receive bidi streams from it.
type ProtoConn interface {
	// CloseWithReason closes the connection with the specified reason.
	// It will try to send a close message to the other side, but delivery is not guaranteed.
	CloseWithReason(string) error

	// OpenBidiWithMsg opens a new bidirectional stream and sends the specified protocol message on it.
	// It is the responsibility of the caller to close the bidi after it is opened successfully.
	OpenBidiWithMsg(typ pb.MsgType, msg proto.Message) (bidi ProtoBidi, err error)

	// WaitForBidi waits for a new bidirectional stream and returns when one is received.
	WaitForBidi(ctx context.Context) (ProtoBidi, error)

	// SendAndReceive is like OpenBidiWithMsg but immediately receives a reply and closes the bidi afterward.
	//
	// If you know what type you are expecting, SendAndReceiveExpect is a better alternative.
	SendAndReceive(typ pb.MsgType, msg proto.Message) (*UntypedProtoMsg, error)
}

// ProtoConnImpl wraps a QUIC connection to provide protocol-specific methods.
type ProtoConnImpl struct {
	// The underlying QUIC connection.
	Inner *quic.Conn
}

var _ ProtoConn = &ProtoConnImpl{}

// ToProtoConn wraps a QUIC connection to provide protocol-specific methods.
func ToProtoConn(conn *quic.Conn) ProtoConn {
	return &ProtoConnImpl{conn}
}

func (conn *ProtoConnImpl) CloseWithReason(reason string) error {
	return conn.Inner.CloseWithError(0, reason)
}

func (conn *ProtoConnImpl) OpenBidiWithMsg(typ pb.MsgType, msg proto.Message) (bidi ProtoBidi, err error) {
	stream, err := conn.Inner.OpenStream()
	if err != nil {
		return ProtoBidi{}, fmt.Errorf(`failed to open bidi before writing message of type %s: %w`, typ.String(), err)
	}

	bidi = wrapBidi(stream)

	err = bidi.Write(typ, msg)
	if err != nil {
		_ = bidi.Close()
		return ProtoBidi{}, err
	}

	return bidi, nil
}

func (conn *ProtoConnImpl) WaitForBidi(ctx context.Context) (ProtoBidi, error) {
	stream, err := conn.Inner.AcceptStream(ctx)
	if err != nil {
		return ProtoBidi{}, fmt.Errorf(`failed to accept stream in WaitForBidi: %w`, err)
	}

	return wrapBidi(stream), nil
}

func (conn *ProtoConnImpl) SendAndReceive(typ pb.MsgType, msg proto.Message) (*UntypedProtoMsg, error) {
	bidi, err := conn.OpenBidiWithMsg(typ, msg)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	return bidi.Read()
}

// SendAndReceiveExpect is like SendAndReceive but also checks that the reply's type matches the expected type.
// See ReadExpect for important details, as it works the same way.
func SendAndReceiveExpect[T proto.Message](
	conn ProtoConn,
	typ pb.MsgType,
	msg proto.Message,
	expectType pb.MsgType,
) (*TypedProtoMsg[T], error) {
	reply, err := conn.SendAndReceive(typ, msg)
	if err != nil {
		return nil, err
	}

	if reply.Type != expectType {
		return nil, fmt.Errorf("unexpected reply type: got %v, expected %v", reply.Type, expectType)
	}

	casted, ok := reply.Payload.(T)
	if !ok {
		wantType := reflect.TypeFor[T]()
		gotType := reflect.TypeOf(msg)

		panic(fmt.Sprintf(`BUG: got message of type %s (struct %s) as reply to new bidi with message type %s but tried to cast it to struct %s`,
			reply.Type.String(),
			gotType.String(),
			typ.String(),
			wantType.String(),
		))
	}

	return &TypedProtoMsg[T]{
		Type:    reply.Type,
		Payload: casted,
	}, nil
}

type BidiHandler func(conn *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error

// ProtoStreamReader wraps a QUIC receive stream to read protocol messages.
// It does not manage the stream lifecycle or hijack it in any way;
// the caller can read raw data from the stream if they need to.
type ProtoStreamReader struct {
	stream io.Reader
}

func NewProtoStreamReader(stream io.Reader) *ProtoStreamReader {
	return &ProtoStreamReader{
		stream: stream,
	}
}

// ReadRaw tries to read a protocol message from the stream.
// It does not do any special handling for error types.
// If the bidi was closed because a remote peer was unreachable, returns ErrPeerUnreachable.
func (r *ProtoStreamReader) ReadRaw() (*UntypedProtoMsg, error) {
	var (
		n   int
		err error
	)

	// Read header.
	// A tiny read like this is fine because QUIC streams are buffered.
	var header [msgHeaderSize]byte
	headerRead := 0
	for headerRead < len(header) {
		n, err = r.stream.Read(header[headerRead:])
		if n > 0 {
			headerRead += n
		}
		if err != nil {
			var streamErr *quic.StreamError
			if errors.As(err, &streamErr) {
				if streamErr.ErrorCode == ProxyPeerUnreachableStreamErrorCode {
					return nil, ErrPeerUnreachable
				}
			}

			if err == io.EOF && headerRead == len(header) {
				break
			}
			return nil, fmt.Errorf(`failed to read protocol message header: %w`, err)
		}
	}

	typ := pb.MsgType(binary.LittleEndian.Uint32(header[:4]))
	payloadLen := binary.LittleEndian.Uint32(header[4:])

	// TODO Enforce max payload size.

	// Read payload.
	readSize := 0
	payload := make([]byte, payloadLen)
	for readSize < len(payload) {
		n, err = r.stream.Read(payload[readSize:])
		if n > 0 {
			readSize += n
		}
		if err != nil {
			if err == io.EOF && readSize == len(payload) {
				break
			}
			return nil, fmt.Errorf(`got protocol message header with type %s and length %d, but failed reading payload at %d bytes: %w`,
				typ.String(),
				payloadLen,
				readSize,
				err,
			)
		}
	}

	// Decode message.
	msg := MsgTypeToEmptyMsg(typ)

	// TODO If msg is nil, return error

	err = proto.Unmarshal(payload, msg)
	if err != nil {
		return nil, fmt.Errorf(`failed to decode protocol message payload with supposed type %s and length %d: %w`,
			typ.String(),
			payloadLen,
			err,
		)
	}

	return &UntypedProtoMsg{
		Type:    typ,
		Payload: msg,
	}, nil
}

// ReadRaw tries to read a protocol message from the stream.
// If the type was MSG_TYPE_ERROR, returns a ProtoMsgError.
//
// If you know what type you are expecting, ReadExpect is a better alternative.
func (r *ProtoStreamReader) Read() (*UntypedProtoMsg, error) {
	msg, err := r.ReadRaw()
	if err != nil {
		return nil, err
	}

	if msg.Type == pb.MsgType_MSG_TYPE_ERROR {
		errMsg := msg.Payload.(*pb.MsgError)

		return nil, NewProtoMsgError(errMsg)
	}

	return msg, nil
}

// ReadExpect tries to read a protocol message from the stream.
// If the type was MSG_TYPE_ERROR, returns a ProtoMsgError.
// Otherwise, if the message type does not match the expected type, it returns an UnexpectedMsgTypeError.
//
// It is extremely important that the generic type on this function is appropriate for the expected type.
// If the generic type does not correspond to the expected type, the function will panic.
func ReadExpect[T proto.Message](r *ProtoStreamReader, expectedType pb.MsgType) (*TypedProtoMsg[T], error) {
	msg, err := r.Read()
	if err != nil {
		return nil, err
	}

	if msg.Type != expectedType {
		return nil, NewUnexpectedMsgTypeError(expectedType, msg.Type)
	}

	casted, ok := msg.Payload.(T)
	if !ok {
		wantType := reflect.TypeFor[T]()
		gotType := reflect.TypeOf(msg)

		panic(fmt.Sprintf(`BUG: got message of type %s (struct %s) but tried to cast it to struct %s`,
			msg.Type.String(),
			gotType.String(),
			wantType.String(),
		))
	}

	return &TypedProtoMsg[T]{
		Type:    expectedType,
		Payload: casted,
	}, nil
}

// ProtoStreamWriter wraps a QUIC receive stream to write protocol messages.
// It does not manage the stream lifecycle or hijack it in any way;
// the caller can write raw data to the stream if they need to.
// If the bidi was closed because a remote peer was unreachable, returns ErrPeerUnreachable.
type ProtoStreamWriter struct {
	stream io.Writer
}

func NewProtoStreamWriter(stream io.Writer) *ProtoStreamWriter {
	return &ProtoStreamWriter{
		stream: stream,
	}
}

// Write tries to write a protocol message to the stream.
func (w *ProtoStreamWriter) Write(typ pb.MsgType, msg proto.Message) error {
	msgSize := proto.Size(msg)
	msgBuf := make([]byte, msgHeaderSize, msgHeaderSize+msgSize)

	// Write header.
	binary.LittleEndian.PutUint32(msgBuf[:4], uint32(typ))
	binary.LittleEndian.PutUint32(msgBuf[4:8], uint32(msgSize))

	// Marshal and append payload.
	var err error
	msgBuf, err = proto.MarshalOptions{}.MarshalAppend(msgBuf, msg)
	if err != nil {
		return fmt.Errorf(`failed to marshal payload for message with type %s: %w`,
			typ.String(),
			err,
		)
	}

	// Write message.
	written := 0
	for written < len(msgBuf) {
		n, err := w.stream.Write(msgBuf[written:])
		if err != nil {
			var streamErr *quic.StreamError
			if errors.As(err, &streamErr) {
				if streamErr.ErrorCode == ProxyPeerUnreachableStreamErrorCode {
					return ErrPeerUnreachable
				}
			}

			return fmt.Errorf(`failed to write payload for message type %s while %d bytes in: %w`,
				typ.String(),
				written,
				err,
			)
		}

		written += n
	}

	return nil
}

// ProtoBidi is a wrapper around a QUIC bidirectional stream with a protocol reader and writer.
type ProtoBidi struct {
	Stream *quic.Stream
	*ProtoStreamReader
	*ProtoStreamWriter
}

// Close closes the send side and cancels the read side to fully release the stream.
func (bidi ProtoBidi) Close() error {
	_ = bidi.Stream.Close()
	bidi.Stream.CancelRead(0)
	return nil
}

func wrapBidi(stream *quic.Stream) ProtoBidi {
	return ProtoBidi{
		Stream:            stream,
		ProtoStreamReader: NewProtoStreamReader(stream),
		ProtoStreamWriter: NewProtoStreamWriter(stream),
	}
}

// WriteAck writes an acknowledgement message to the bidi stream.
func (bidi ProtoBidi) WriteAck() error {
	return bidi.Write(pb.MsgType_MSG_TYPE_ACKNOWLEDGED, &pb.MsgAcknowledged{})
}

// WriteError writes an error message to the bidi stream.
// If the message is empty, it will be sent as nil.
func (bidi ProtoBidi) WriteError(typ pb.ErrType, msg string) error {
	return bidi.Write(pb.MsgType_MSG_TYPE_ERROR, &pb.MsgError{
		Type:    typ,
		Message: common.StrOrNil(msg),
	})
}

// WriteFileNotExistError writes an ERR_TYPE_FILE_NOT_EXIST error to the bidi stream,
// based on the specified path.
func (bidi ProtoBidi) WriteFileNotExistError(path string) error {
	return bidi.WriteError(pb.ErrType_ERR_TYPE_FILE_NOT_EXIST, fmt.Sprintf("no such path %q", path))
}

// WriteUnexpectedMsgTypeError writes an ERR_TYPE_UNEXPECTED_MSG_TYPE error to the bidi stream,
// based on the specified expected and actual message types.
func (bidi ProtoBidi) WriteUnexpectedMsgTypeError(expected pb.MsgType, actual pb.MsgType) error {
	return bidi.WriteError(
		pb.ErrType_ERR_TYPE_UNEXPECTED_MSG_TYPE,
		fmt.Sprintf("expected %s but got %s", expected.String(), actual.String()),
	)
}

// WriteInternalError writes an ERR_TYPE_INTERNAL error to the bidi stream.
// Uses the error message from the specified error, or a placeholder if it is nil.
func (bidi ProtoBidi) WriteInternalError(errOrNil error) error {
	var message string
	if errOrNil == nil {
		message = "internal error"
	} else {
		message = errOrNil.Error()
	}
	return bidi.WriteError(pb.ErrType_ERR_TYPE_INTERNAL, message)
}

// WriteUnimplementedError writes an ERR_TYPE_UNIMPLEMENTED error to the bidi stream,
// based on the specified message type.
func (bidi ProtoBidi) WriteUnimplementedError(msgType pb.MsgType) error {
	return bidi.WriteError(
		pb.ErrType_ERR_TYPE_UNIMPLEMENTED,
		fmt.Sprintf("handler for %q is unimplemented", msgType.String()),
	)
}

// CompareProtoVersions compares two protocol versions.
// If the two versions are identical, returns 0.
// If version `a` is newer, returns 1.
// If version `b` is newer, returns -1.
func CompareProtoVersions(a *pb.ProtoVersion, b *pb.ProtoVersion) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}

	return 0
}

// ProtoListener represents a listener that can accept protocol connections.
type ProtoListener interface {
	io.Closer

	// Accept accepts a new protocol connection.
	Accept(context.Context) (ProtoConn, error)
}

// QuicProtoListener implements ProtoListener using QUIC.
type QuicProtoListener struct {
	*quic.Listener
}

func (l *QuicProtoListener) Close() error {
	return l.Listener.Close()
}

func (l *QuicProtoListener) Accept(ctx context.Context) (ProtoConn, error) {
	conn, err := l.Listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	return ToProtoConn(conn), nil
}

// NewQuicProtoListener creates a ProtoListener on the specified address and with the specified TLS config.
func NewQuicProtoListener(listenAddr string, tlsCfg *tls.Config) (ProtoListener, error) {
	addrPort, err := netip.ParseAddrPort(listenAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to parse listen address %q: %w`, listenAddr, err)
	}

	var udpConn *net.UDPConn
	addr := addrPort.Addr()
	if addr.Is6() {
		udpConn, err = net.ListenUDP("udp6", &net.UDPAddr{
			IP:   addr.AsSlice(),
			Port: int(addrPort.Port()),
		})
	} else {
		udpConn, err = net.ListenUDP("udp4", &net.UDPAddr{
			IP:   addr.AsSlice(),
			Port: int(addrPort.Port()),
		})
	}
	if err != nil {
		return nil, err
	}

	trans := quic.Transport{Conn: udpConn}
	listener, err := trans.Listen(tlsCfg, &quic.Config{
		KeepAlivePeriod: DefaultKeepAlivePeriod,
	})
	if err != nil {
		return nil, err
	}

	return &QuicProtoListener{listener}, nil
}
