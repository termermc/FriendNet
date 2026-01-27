package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

type UntypedProtoMsg struct {
	Type    pb.MsgType
	Payload proto.Message
}

type TypedProtoMsg[T proto.Message] struct {
	Type    pb.MsgType
	Payload T
}

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

	return &TypedProtoMsg[T]{
		Type:    msg.Type,
		Payload: casted,
	}
}

type BidiHandler func(conn *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error

const msgHeaderSize = 8

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
func (r *ProtoStreamReader) ReadRaw() (*UntypedProtoMsg, error) {
	// Read header.
	// A tiny read like this is fine because QUIC streams are buffered.
	var header [msgHeaderSize]byte
	n, err := r.stream.Read(header[:])
	if err != nil {
		return nil, fmt.Errorf(`failed to read protocol message header: %w`, err)
	}
	if n < len(header) {
		return nil, fmt.Errorf(`was only able to read %d bytes of %d byte protocol message header`, n, len(header))
	}

	typ := pb.MsgType(binary.LittleEndian.Uint32(header[:4]))
	payloadLen := binary.LittleEndian.Uint32(header[4:])

	// TODO Enforce max payload size.

	// Read payload.
	readSize := 0
	payload := make([]byte, payloadLen)
	for readSize < len(payload) {
		n, err = r.stream.Read(payload[readSize:])
		if err != nil {
			return nil, fmt.Errorf(`got protocol message header with type %s and length %d, but failed reading payload at %d bytes: %w`,
				typ.String(),
				payloadLen,
				readSize,
				err,
			)
		}
		readSize += n
	}

	// Decode message.
	msg := MsgTypeToEmptyMsg(typ)
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
func ReadExpect[T proto.Message](r *ProtoStreamReader, expectedType pb.MsgType) (T, error) {
	var empty T

	msg, err := r.Read()
	if err != nil {
		return empty, err
	}

	if msg.Type != expectedType {
		return empty, NewUnexpectedMsgTypeError(expectedType, msg.Type)
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

	return casted, nil
}

// ProtoStreamWriter wraps a QUIC receive stream to write protocol messages.
// It does not manage the stream lifecycle or hijack it in any way;
// the caller can write raw data to the stream if they need to.
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
	msgBuf := make([]byte, 0, msgHeaderSize+msgSize)

	// Write header.
	binary.LittleEndian.PutUint32(msgBuf[:4], uint32(typ))
	binary.LittleEndian.PutUint32(msgBuf[4:8], uint32(msgSize))

	// Marshal and append payload.
	var err error
	msgBuf, err = proto.MarshalOptions{}.MarshalAppend(msgBuf[msgHeaderSize:], msg)
	if err != nil {
		return fmt.Errorf(`failed to marshal payload for message with type %s: %w`,
			typ.String(),
			err,
		)
	}

	// Write message.
	written := 0
	for written < len(msgBuf) {
		n, err := w.stream.Write(msgBuf)
		if err != nil {
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

func wrapBidi(stream *quic.Stream) ProtoBidi {
	return ProtoBidi{
		Stream:            stream,
		ProtoStreamReader: NewProtoStreamReader(stream),
		ProtoStreamWriter: NewProtoStreamWriter(stream),
	}
}

// OpenBidiWithMsg opens a new bidirectional stream and sends the specified protocol message on it.
func OpenBidiWithMsg(conn *quic.Conn, typ pb.MsgType, msg proto.Message) (bidi ProtoBidi, err error) {
	stream, err := conn.OpenStream()
	if err != nil {
		return ProtoBidi{}, fmt.Errorf(`failed to open bidi before writing message of type %s: %w`, typ.String(), err)
	}

	bidi = wrapBidi(stream)

	err = bidi.Write(typ, msg)
	if err != nil {
		return ProtoBidi{}, err
	}

	return bidi, nil
}

// WaitForBidi waits for a new bidirectional stream and returns when one is received.
func WaitForBidi(ctx context.Context, conn *quic.Conn) (ProtoBidi, error) {
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return ProtoBidi{}, fmt.Errorf(`failed to accept stream in WaitForBidi: %w`, err)
	}

	return wrapBidi(stream), nil
}

// HandleBidiRequest waits for a new bidirectional stream, launches a goroutine to handle the stream, then returns immediately.
// The new goroutine reads the first message, chooses a handler based on the MsgType, then calls it.
// If the message's type is found in the handlers map, that handler is called.
// If no appropriate handler is found, fallback is called (if not nil).
// Any errors that occur during the first message read or calling the handler will call errorHandler.
//
// Important: handlers must never be modified after being passed to this function.
func HandleBidiRequest(ctx context.Context, conn *quic.Conn, handlers map[pb.MsgType]BidiHandler, fallback BidiHandler, errorHandler func(error)) error {
	bidi, bidiErr := WaitForBidi(ctx, conn)
	if bidiErr != nil {
		return fmt.Errorf(`failed to wait for bidi in HandleBidiRequest: %w`, bidiErr)
	}

	go func() {
		// TODO Handle timeout.

		// Read first message.
		msg, err := bidi.Read()
		if err != nil {
			errorHandler(fmt.Errorf(`failed to read first proto message in HandleBidiRequest: %w`, err))
		}

		// Choose appropriate handler.
		handler, has := handlers[msg.Type]
		if has {
			err = handler(conn, bidi, msg)
		} else if fallback != nil {
			err = fallback(conn, bidi, msg)
		} else {
			// TODO Log unexpected message type.
			err = nil
		}
		if err != nil {
			errorHandler(err)
		}
	}()

	return nil
}
