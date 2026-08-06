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

// Channel that is never closed or written, to satisfy ProtoConn.OnDisconnect.
var virtualC2cConnOnDiscChan = make(chan struct{})

// VirtualC2cConnMode is a type of virtual C2C connection mode.
type C2cConnMode int

const (
	// C2cConnModeDefault is the default mode for virtual C2C connections.
	// It tries to direct connect if not already connected, and if it times out, it finally falls back to proxied.
	C2cConnModeDefault C2cConnMode = iota

	// C2cConnModeAlwaysProxy will always use the proxy in place of connecting directly.
	C2cConnModeAlwaysProxy

	// C2cConnModeQuickFallback will instantly fall back on the proxy if no existing connection exists.
	// If none exists, it will try to kick off a direct connection in the background.
	C2cConnModeQuickFallback
)

// VirtualC2cConn is a virtual connection to another client.
// It is stateless and does not manage any direct or proxied connections.
// It exists to implement protocol.ProtoConn.
type VirtualC2cConn struct {
	// The underlying server connection.
	ServerConn *Conn

	// The client's username.
	Username common.NormalizedUsername

	// The connection mode to use.
	ConnMode C2cConnMode
}

// OnDisconnect returns a channel that never closes for VirtualC2cConn.
func (c VirtualC2cConn) OnDisconnect() <-chan struct{} {
	return virtualC2cConnOnDiscChan
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

	// If C2C points to ourselves, return a loopback bidi stream.
	if c.Username == c.ServerConn.Username {
		bidi1, bidi2 := protocol.NewPipeProtoBidi()
		c.ServerConn.incomingBidi <- C2cBidi{
			ProtoBidi: bidi2,
			RoomConn:  c.ServerConn,
			Username:  c.Username,
		}

		return bidi1, bidi1.Write(typ, msg)
	}

	// Do a normal C2C message.
	return c.ServerConn.openC2cBidiWithMsg(c.Username, typ, msg, c.ConnMode)
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
		return protocol.NewUnexpectedMsgTypeError(
			pb.MsgType_MSG_TYPE_ACKNOWLEDGED,
			reply.Type,
			reply.Payload,
		)
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
		bidi,
		pb.MsgType_MSG_TYPE_FILE_META,
	)
	if err != nil {
		return nil, nil, err
	}

	// Now that we have the metadata, we can treat the bidi as a binary stream.
	reader = common.NewLimitReadCloser(
		protocol.NewReadCloserWithFunc(bidi.RawReader(), bidi.Close),
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
