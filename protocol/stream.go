package protocol

import (
	"errors"
	"io"

	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

// Stream is an interface that defines a pull-based stream for any type of value.
type Stream[T any] interface {
	// ReadNext reads the next value from the stream.
	// Returns io.EOF when the stream has ended.
	// Any later calls to Read after the stream has ended will continue to return io.EOF.
	ReadNext() (T, error)
}

// TypedMsgStream is a stream that reads protocol messages of a specific type.
type TypedMsgStream[T proto.Message] struct {
	typ  pb.MsgType
	bidi ProtoBidi
}

func NewTypedMsgStream[T proto.Message](reader ProtoBidi, typ pb.MsgType) TypedMsgStream[T] {
	return TypedMsgStream[T]{
		bidi: reader,
		typ:  typ,
	}
}

// ReadNext reads the next message from the stream.
// If the stream has ended, returns io.EOF.
func (s TypedMsgStream[T]) ReadNext() (*TypedProtoMsg[T], error) {
	msg, err := ReadExpect[T](s.bidi.ProtoStreamReader, s.typ)
	if err != nil {
		var streamErr *quic.StreamError
		if errors.As(err, &streamErr) ||
			errors.Is(err, quic.ErrServerClosed) ||
			errors.Is(err, quic.ErrTransportClosed) {
			_ = s.bidi.Close()
			return nil, io.EOF
		}
	}

	return msg, err
}

// Close closes the stream and the underlying bidi.
func (s TypedMsgStream[T]) Close() error {
	return s.bidi.Close()
}

// TransformerStream wraps a stream and applies a transformation function to each value read from the stream.
type TransformerStream[T, R any] struct {
	stream Stream[T]
	fn     func(T) R
}

func NewTransformerStream[T, R any](stream Stream[T], fn func(T) R) TransformerStream[T, R] {
	return TransformerStream[T, R]{
		stream: stream,
		fn:     fn,
	}
}

func (s TransformerStream[T, R]) ReadNext() (R, error) {
	val, err := s.stream.ReadNext()
	return s.fn(val), err
}

// ReadCloserWithFunc wraps an io.Reader and a function to close it.
type ReadCloserWithFunc struct {
	reader io.Reader
	closer func() error
}

func NewReadCloserWithFunc(reader io.Reader, closer func() error) ReadCloserWithFunc {
	return ReadCloserWithFunc{reader: reader, closer: closer}
}

func (r ReadCloserWithFunc) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r ReadCloserWithFunc) Close() error {
	return r.closer()
}
