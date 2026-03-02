package room

import (
	"errors"
	"runtime/debug"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// C2cBidi is a client-to-client bidi stream
type C2cBidi struct {
	// The function to disown the connection.
	// Should be nil if from a proxy.
	disown func()

	protocol.ProtoBidi

	// The associated room connection.
	RoomConn *Conn

	// The client's username.
	Username common.NormalizedUsername
}

// DisownConn disowns the direct connection that created the bidi.
// If the bidi is proxied, this is no-op.
func (b C2cBidi) DisownConn() {
	if b.disown != nil {
		b.disown()
	}
}

func (c *Conn) c2cLoop() {
loop:
	for {
		select {
		case <-c.Context.Done():
			break loop
		case bidi := <-c.incomingBidi:
			go func() {
				defer func() {
					if err := recover(); err != nil {
						c.logger.Error("c2c bidi handler panic",
							"service", "room.Conn",
							"err", err,
							"stack", string(debug.Stack()),
						)
					}
				}()
				defer func() {
					_ = bidi.Close()
				}()

				rawMsg, err := bidi.Read()
				if err != nil {
					if protocol.IsErrorConnCloseOrCancel(err) {
						return
					}
					if _, ok := errors.AsType[*quic.StreamError](err); ok {
						return
					}

					c.logger.Error("failed to read c2c bidi message",
						"service", "room.Conn",
						"err", err,
					)
					return
				}

				// Handle C2C message.
				err = nil
				switch rawMsg.Type {
				case pb.MsgType_MSG_TYPE_BYE:
					_ = bidi.WriteAck()
					bidi.DisownConn()
					err = nil
				case pb.MsgType_MSG_TYPE_PING:
					err = c.logic.OnPing(c.Context, c, bidi.ProtoBidi, protocol.ToTyped[*pb.MsgPing](rawMsg))
				case pb.MsgType_MSG_TYPE_GET_DIR_FILES:
					err = c.logic.OnGetDirFiles(c.Context, c, bidi, protocol.ToTyped[*pb.MsgGetDirFiles](rawMsg))
				case pb.MsgType_MSG_TYPE_GET_FILE_META:
					err = c.logic.OnGetFileMeta(c.Context, c, bidi, protocol.ToTyped[*pb.MsgGetFileMeta](rawMsg))
				case pb.MsgType_MSG_TYPE_GET_FILE:
					err = c.logic.OnGetFile(c.Context, c, bidi, protocol.ToTyped[*pb.MsgGetFile](rawMsg))
				case pb.MsgType_MSG_TYPE_CONNECT_TO_ME:
					err = c.logic.OnConnectToMe(c.Context, c, bidi, protocol.ToTyped[*pb.MsgConnectToMe](rawMsg))
				case pb.MsgType_MSG_TYPE_SEARCH:
					err = c.logic.OnSearch(c.Context, c, bidi.ProtoBidi, protocol.ToTyped[*pb.MsgSearch](rawMsg))
				default:
					err = bidi.WriteUnimplementedError(rawMsg.Type)
				}
				if err != nil {
					c.logger.Error("failed to handle C2C bidi message",
						"service", "room.Conn",
						"type", rawMsg.Type.String(),
						"err", err,
					)
				}
			}()
		}
	}
}
