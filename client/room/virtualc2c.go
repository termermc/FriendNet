package room

import (
	"context"
	"errors"
	"io"
	"net"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"google.golang.org/protobuf/proto"
)

// VirtualC2cConn is a virtual connection to another client.
// It is stateless and does not manage any direct or proxied connections.
// It exists to implement protocol.ProtoConn.
type VirtualC2cConn struct {
	// The underlying server connection.
	ServerConn *Conn

	// The client's username.
	Username common.NormalizedUsername

	// Whether to force proxying instead of using a direct connection.
	// It may still fall back to proxying if no direct connection method is available.
	ForceProxy bool
}

func (c VirtualC2cConn) lockCheck() error {
	c.ServerConn.mu.RLock()
	defer c.ServerConn.mu.RUnlock()
	if c.ServerConn.isClosed {
		return ErrRoomConnClosed
	}
	return nil
}

// RemoteAddr is no-op.
func (c VirtualC2cConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0, Zone: ""}
}

// CloseWithReason is no-op.
func (c VirtualC2cConn) CloseWithReason(string) error {
	return nil
}

func (c VirtualC2cConn) OpenBidiWithMsg(typ pb.MsgType, msg proto.Message) (bidi protocol.ProtoBidi, err error) {
	if err = c.lockCheck(); err != nil {
		return
	}

	return c.ServerConn.openC2cBidiWithMsg(c.Username, typ, msg, c.ForceProxy)
}

func (c VirtualC2cConn) WaitForBidi(ctx context.Context) (protocol.ProtoBidi, error) {
	return protocol.ProtoBidi{}, errors.New("not implemented by VirtualC2cConn")
}

func (c VirtualC2cConn) SendAndReceive(typ pb.MsgType, msg proto.Message) (*protocol.UntypedProtoMsg, error) {
	bidi, err := c.OpenBidiWithMsg(typ, msg)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	return bidi.Read()
}

func (c VirtualC2cConn) SendAndReceiveAck(typ pb.MsgType, msg proto.Message) error {
	reply, err := c.SendAndReceive(typ, msg)
	if err != nil {
		return err
	}

	if reply.Type != pb.MsgType_MSG_TYPE_ACKNOWLEDGED {
		return protocol.UnexpectedMsgTypeError{
			Expected: pb.MsgType_MSG_TYPE_ACKNOWLEDGED,
			Actual:   reply.Type,
		}
	}

	return nil
}

var _ protocol.ProtoConn = VirtualC2cConn{}

// GetDirFiles returns a stream of files in the specified directory.
func (c VirtualC2cConn) GetDirFiles(path common.ProtoPath) (protocol.Stream[*pb.MsgDirFiles], error) {
	bidi, err := c.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_GET_DIR_FILES, &pb.MsgGetDirFiles{
		Path: path.String(),
	})
	if err != nil {
		return nil, err
	}

	return protocol.NewTransformerStream(
		protocol.NewTypedMsgStream[*pb.MsgDirFiles](bidi, pb.MsgType_MSG_TYPE_DIR_FILES),
		func(msg *protocol.TypedProtoMsg[*pb.MsgDirFiles]) *pb.MsgDirFiles {
			return msg.Payload
		},
	), nil
}

// GetFileMeta returns the metadata of the specified file.
func (c VirtualC2cConn) GetFileMeta(path common.ProtoPath) (*pb.MsgFileMeta, error) {
	msg, err := protocol.SendAndReceiveExpect[*pb.MsgFileMeta](
		c,
		pb.MsgType_MSG_TYPE_GET_FILE_META,
		&pb.MsgGetFileMeta{
			Path: path.String(),
		},
		pb.MsgType_MSG_TYPE_FILE_META,
	)
	if err != nil {
		return nil, err
	}

	return msg.Payload, nil
}

// GetFile returns the metadata for the specified file, and then a stream of its data.
// If the file is empty or is a directory, the stream will always return io.EOF.
//
// It is up to the caller to enforce timeouts.
func (c VirtualC2cConn) GetFile(req *pb.MsgGetFile) (meta *pb.MsgFileMeta, reader io.ReadCloser, err error) {
	bidi, err := c.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_GET_FILE, req)
	if err != nil {
		return nil, nil, err
	}

	msg, err := protocol.ReadExpect[*pb.MsgFileMeta](
		bidi.ProtoStreamReader,
		pb.MsgType_MSG_TYPE_FILE_META,
	)
	if err != nil {
		return nil, nil, err
	}

	// Now that we have the metadata, we can treat the bidi as a binary stream.
	reader = common.NewLimitReadCloser(
		protocol.NewReadCloserWithFunc(bidi.Stream, bidi.Close),
		int64(msg.Payload.Size),
	)
	return msg.Payload, reader, nil
}

// Search returns a stream of search results for the specified query.
func (c VirtualC2cConn) Search(query string) (protocol.Stream[*pb.MsgSearchResult], error) {
	bidi, err := c.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_SEARCH, &pb.MsgSearch{
		Query: query,
	})
	if err != nil {
		return nil, err
	}

	return protocol.NewTransformerStream(
		protocol.NewTypedMsgStream[*pb.MsgSearchResult](bidi, pb.MsgType_MSG_TYPE_SEARCH_RESULT),
		func(msg *protocol.TypedProtoMsg[*pb.MsgSearchResult]) *pb.MsgSearchResult {
			return msg.Payload
		},
	), nil
}
