package i2pcompat

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"strconv"
	"time"

	"friendnet.org/protocol/compat"
	sam3 "github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/stream"
	"github.com/go-i2p/i2pkeys"
)

// I2pManager creates a SAM connection and manages I2P connections.
// It uses compat.ConnManager to emulate QUIC semantics over I2P streams.
type I2pManager struct {
	sam      *sam3.SAM
	keys     i2pkeys.I2PKeys
	listener *stream.StreamListener

	cm *compat.ConnManager
}

// NewI2pManager creates a new SAM client and streams session for a new I2pManager.
// It returns an error if creating the SAM client or streams session fails.
func NewI2pManager(
	logger *slog.Logger,
	samAddr string,
	connInactivityTimeout time.Duration,
) (*I2pManager, error) {
	logger.Debug("creating SAM client",
		"service", "i2pcompat.I2pManager",
	)

	sam, err := sam3.NewSAM(samAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to create SAM client: %w`, err)
	}

	keys, err := sam.NewKeys()
	if err != nil {
		return nil, fmt.Errorf(`failed to generate I2P keys: %w`, err)
	}

	logger.Debug("creating stream session",
		"addr", keys.Addr().String(),
		"service", "i2pcompat.I2pManager",
	)

	sess, err := stream.NewStreamSession(
		sam.SAM,
		"i2pcompat"+strconv.FormatInt(rand.Int64(), 10),
		keys,
		sam3.Options_Default,
	)
	if err != nil {
		_ = sam.Close()
		return nil, fmt.Errorf(`failed to create stream session: %w`, err)
	}

	logger.Debug("stream session created",
		"addr", keys.Addr().String(),
		"service", "i2pcompat.I2pManager",
	)

	listener, err := sess.Listen()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf(`failed to create incoming stream listener: %w`, err)
	}

	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		return sess.DialContext(ctx, addr)
	}
	accept := func() (net.Conn, error) {
		return listener.Accept()
	}

	cm := compat.NewConnManager(
		logger,
		sam,
		dial,
		accept,
		connInactivityTimeout,
	)

	return &I2pManager{
		sam:      sam,
		keys:     keys,
		listener: listener,

		cm: cm,
	}, nil
}

// Close closes the I2P manager and all owned connections.
func (i *I2pManager) Close() error {
	_ = i.cm.Close()
	_ = i.listener.Close()
	_ = i.sam.Close()
	return nil
}

// Addr returns the I2P address of the I2P manager.
func (i *I2pManager) Addr() net.Addr {
	return i.keys.Addr()
}

// Accept waits for a new incoming connection and returns it.
// Returns compat.ErrConnManagerClosed if the I2pManager or its underlying compat.ConnManager is closed.
func (i *I2pManager) Accept(ctx context.Context) (*compat.Conn, error) {
	return i.cm.Accept(ctx)
}

// Dial makes a new outgoing connection to the specified address.
func (i *I2pManager) Dial(ctx context.Context, addr string) (*compat.Conn, error) {
	return i.cm.Dial(ctx, addr)
}
