package room

import (
	"errors"
	"io"

	"friendnet.org/common"
	"friendnet.org/protocol"
	"github.com/quic-go/quic-go"
)

// C2cBidi is a client-to-client bidi stream
type C2cBidi struct {
	protocol.ProtoBidi

	// The associated room connection.
	RoomConn *Conn

	// The client's username.
	Username common.NormalizedUsername
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
						)
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

					c.logger.Error("failed to read c2c bidi message",
						"service", "room.Conn",
						"err", err,
					)
					return
				}

				// Handle C2C message.
				err = nil
				switch rawMsg.Type {
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

				_ = bidi.Close()
			}()
		}
	}
}
