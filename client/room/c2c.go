package room

import (
	"log/slog"

	"friendnet.org/common"
	"friendnet.org/protocol"
)

// C2cBidi is a client-to-client bidi stream
type C2cBidi struct {
	protocol.ProtoBidi

	// The client's username.
	Username common.NormalizedUsername

	// The associated room connection.
	RoomCoon *Conn
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
						c.logger.Error("c2c bidi handler panic", slog.Any("err", err))
					}
				}()

				// Read message.
				msg, err := bidi.Read()
				if err != nil {
					c.logger.Error("failed to read bidi message", slog.Any("err", err))
					return
				}

				// TODO Handle message.
				err = nil
				switch payload := msg.Payload.(type) {
				default:
					_ = bidi.WriteUnimplementedError(msg.Type)

					// TODO REMOVE THIS
					_ = payload
				}
				if err != nil {
					c.logger.Error("failed to handle bidi message",
						"service", "room.Conn",
						"type", msg.Type.String(),
						"err", err,
					)
				}

				_ = bidi.Close()
			}()
		}
	}
}
