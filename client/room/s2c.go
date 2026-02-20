package room

import (
	"context"
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
			var idleErr *quic.IdleTimeoutError
			var appErr *quic.ApplicationError
			if errors.Is(waitErr, context.Canceled) ||
				errors.Is(waitErr, io.EOF) ||
				errors.As(waitErr, &idleErr) ||
				errors.As(waitErr, &appErr) {
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
				var streamErr *quic.StreamError
				if errors.As(err, &streamErr) {
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
			case pb.MsgType_MSG_TYPE_PING:
				msg := protocol.ToTyped[*pb.MsgPing](rawMsg)
				err = c.logic.OnPing(c.Context, c, bidi, msg)
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
