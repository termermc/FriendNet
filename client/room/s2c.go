package room

import (
	"errors"
	"io"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

func (c *Conn) s2cLoop() {
	for {
		bidi, waitErr := c.serverConn.WaitForBidi(c.Context)
		if waitErr != nil {
			if protocol.IsErrorConnCloseOrCancel(waitErr) {
				return
			}

			c.logger.Error("failed to wait for s2c bidi",
				"service", "room.Conn",
				"err", waitErr,
			)
			return
		}

		go func() {
			cancelBidiClose := false
			defer func() {
				if !cancelBidiClose {
					_ = bidi.Close()
				}
			}()

			rawMsg, err := bidi.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				if _, ok := errors.AsType[*quic.StreamError](err); ok {
					return
				}

				c.logger.Error("failed to read s2c bidi message",
					"service", "room.Conn",
					"err", err,
				)
				return
			}

			// Check if this is an inbound proxy.
			{
				payload, ok := rawMsg.Payload.(*pb.MsgInboundProxy)
				if ok {
					// Pass proxy bidi to C2C chan.
					cancelBidiClose = true
					c.incomingBidi <- C2cBidi{
						ProtoBidi: bidi,
						RoomConn:  c,
						Username:  common.UncheckedCreateNormalizedUsername(payload.OriginUsername),
					}
					return
				}
			}

			// Handle S2C message.
			err = nil
			switch rawMsg.Type {
			case pb.MsgType_MSG_TYPE_BYE:
				c.logger.Info("server shut down",
					"service", "room.Conn",
					"room", c.RoomName.String(),
				)
				_ = bidi.WriteAck()
				_ = c.serverConn.CloseWithReason("it was nice knowing you")
			case pb.MsgType_MSG_TYPE_PING:
				err = c.logic.OnPing(c.Context, c, bidi, protocol.ToTyped[*pb.MsgPing](rawMsg))
			case pb.MsgType_MSG_TYPE_CLIENT_ONLINE:
				err = c.logic.OnClientOnline(c.Context, c, bidi, protocol.ToTyped[*pb.MsgClientOnline](rawMsg))
			case pb.MsgType_MSG_TYPE_CLIENT_OFFLINE:
				err = c.logic.OnClientOffline(c.Context, c, bidi, protocol.ToTyped[*pb.MsgClientOffline](rawMsg))
			case pb.MsgType_MSG_TYPE_SEARCH:
				err = c.logic.OnSearch(c.Context, c, bidi, protocol.ToTyped[*pb.MsgSearch](rawMsg))
			default:
				err = bidi.WriteUnimplementedError(rawMsg.Type)
			}
			if err != nil {
				c.logger.Error("failed to handle S2C bidi message",
					"service", "room.Conn",
					"type", rawMsg.Type.String(),
					"err", err,
				)
			}
		}()
	}
}
