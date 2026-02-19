package room

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// ClientPingInterval is the interval between pings sent to clients.
const ClientPingInterval = 10 * time.Second

// Client is an authenticated client connected to a room.
type Client struct {
	logger *slog.Logger
	conn   protocol.ProtoConn

	version  *pb.ProtoVersion
	Room     *Room
	Username common.NormalizedUsername

	handlers ClientMessageHandlers
}

// NewClient creates a new room client.
func NewClient(
	logger *slog.Logger,
	conn protocol.ProtoConn,

	version *pb.ProtoVersion,
	room *Room,
	username common.NormalizedUsername,

	handlers ClientMessageHandlers,
) *Client {
	return &Client{
		logger: logger,
		conn:   conn,

		version:  version,
		Room:     room,
		Username: username,

		handlers: handlers,
	}
}

func (c *Client) msgHandler(bidi protocol.ProtoBidi, firstMsg *protocol.UntypedProtoMsg) error {
	ctx := context.Background()

	switch firstMsg.Type {
	case pb.MsgType_MSG_TYPE_PING:
		return c.handlers.OnPing(ctx, c, bidi, protocol.ToTyped[*pb.MsgPing](firstMsg))
	case pb.MsgType_MSG_TYPE_OPEN_OUTBOUND_PROXY:
		return c.handlers.OnOpenOutboundProxy(ctx, c, bidi, protocol.ToTyped[*pb.MsgOpenOutboundProxy](firstMsg))
	case pb.MsgType_MSG_TYPE_GET_ONLINE_USERS:
		return c.handlers.OnGetOnlineUsers(ctx, c, bidi, protocol.ToTyped[*pb.MsgGetOnlineUsers](firstMsg))

	default:
		c.logger.Error("client sent unknown message type",
			"service", "room.Client",
			"room", c.Room.Name.String(),
			"username", c.Username.String(),
			"type", firstMsg.Type,
		)

		_ = bidi.WriteUnimplementedError(firstMsg.Type)

		// Don't return an error here.
		// Errors returned are for genuine internal errors.
		return nil
	}
}

func (c *Client) bidiHandler(bidi protocol.ProtoBidi) {
	// Read first message.
	firstMsg, firstErr := bidi.Read()
	if firstErr != nil {
		c.logger.Error("failed to read first message from bidi",
			"service", "room.Client",
			"room", c.Room.Name.String(),
			"username", c.Username.String(),
			"err", firstErr,
		)
		return
	}

	// Wrap message logic handler for better error messages.
	err := c.msgHandler(bidi, firstMsg)
	if err != nil {
		c.logger.Error("failed to handle bidi message",
			"service", "room.Client",
			"room", c.Room.Name.String(),
			"username", c.Username.String(),
			"msg_type", firstMsg.Type.String(),
			"err", err,
		)

		_ = bidi.WriteInternalError(err)
	}
}

// ReadLoop runs the client message read loop.
// Only exits if the room closed, connection closed, a read error occurred, or the client sent an invalid message.
// In any case, the client should be closed once this method returns.
func (c *Client) ReadLoop(ctx context.Context) error {
	for {
		bidi, err := c.conn.WaitForBidi(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}

			return err
		}

		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					c.logger.Error("bidi handler panic",
						"service", "room.Client",
						"room", c.Room.Name.String(),
						"username", c.Username.String(),
						"err", rec,
						"stack", string(debug.Stack()),
					)
				}

				// Handler is finished; close bidi.
				_ = bidi.Close()
			}()

			c.bidiHandler(bidi)
		}()
	}
}

// PingLoop runs the client ping loop.
// Only returns if the context is canceled.
func (c *Client) PingLoop(ctx context.Context) {
	ticker := time.NewTicker(ClientPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := c.Ping(); err != nil {
				var idleErr *quic.IdleTimeoutError
				if errors.As(err, &idleErr) {
					return
				}

				c.logger.Error("failed to ping client",
					"service", "room.Client",
					"room", c.Room.Name.String(),
					"username", c.Username.String(),
					"err", err,
				)
			}
		}
	}
}

// Ping sends a ping request to the client and returns the round-trip time.
func (c *Client) Ping() (time.Duration, error) {
	start := time.Now()
	_, err := c.conn.SendAndReceive(pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
		SentTs: start.UnixMilli(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to send ping to client %q@%q: %w",
			c.Username.String(),
			c.Room.Name.String(),
			err,
		)
	}

	return time.Since(start), nil
}
