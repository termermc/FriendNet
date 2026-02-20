package room

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// ErrTargetNotOnline is returned when trying to open an outbound proxy to a client that is not online.
var ErrTargetNotOnline = errors.New("target client not online")

// ErrProxyClosed is returned when trying to use a closed proxy.
var ErrProxyClosed = errors.New("proxy closed")

// ClientProxy implements a client-to-client proxy through the server.
type ClientProxy struct {
	mu       sync.Mutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	originBidi protocol.ProtoBidi
	targetBidi protocol.ProtoBidi
}

const proxyBufSize = 1024

// NewClientProxy creates a new ClientProxy from an existing origin bidi.
// It assumes that the origin bidi has already had the open request message read from it, meaning the
// only data that will be sent on it will be proxied data.
//
// If the target client is not online, returns ErrTargetNotOnline.
//
// Returns after successfully opening a target bidi and connecting the two clients.
// Call ClientProxy.Run to run the proxy. It can be stopped by calling ClientProxy.Close.
func NewClientProxy(
	room *Room,
	originUsername common.NormalizedUsername,
	targetUsername common.NormalizedUsername,
	originBidi protocol.ProtoBidi,
) (*ClientProxy, error) {
	targetClient, isOnline := room.GetClientByUsername(targetUsername)
	if !isOnline {
		return nil, ErrTargetNotOnline
	}

	proxyBidi, err := targetClient.conn.OpenBidiWithMsg(pb.MsgType_MSG_TYPE_INBOUND_PROXY, &pb.MsgInboundProxy{
		OriginUsername: originUsername.String(),
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to open outbound proxy to from %q to %q: %w`,
			originUsername.String(),
			targetUsername.String(),
			err,
		)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	return &ClientProxy{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		originBidi: originBidi,
		targetBidi: proxyBidi,
	}, nil
}

// Close closes the proxy by closing bidi streams.
// If ClientProxy.Run is currently running, this will cause it to return nil.
// Subsequent calls are no-op.
func (p *ClientProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isClosed {
		return nil
	}
	p.isClosed = true

	p.ctxCancel()

	errs := make([]error, 0, 2)
	if err := p.targetBidi.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := p.originBidi.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("closing proxy bidi streams failed: %w", errors.Join(errs...))
}

func (p *ClientProxy) proxyThread(from protocol.ProtoBidi, to protocol.ProtoBidi) error {
	_, err := io.Copy(to.Stream, from.Stream)
	return err
}

// Run runs the proxy until it is closed.
// Not safe for concurrent use.
// Returns nil once the proxy is closed, either by calling ClientProxy.Close or by either side closing their stream.
func (p *ClientProxy) Run() error {
	p.mu.Lock()
	if p.isClosed {
		p.mu.Unlock()
		return ErrProxyClosed
	}
	p.mu.Unlock()

	// Proxy will be closed after this returns no matter what.
	defer func() {
		_ = p.Close()
	}()

	proxyErr := make(chan error, 1)

	go func() {
		proxyErr <- p.proxyThread(p.originBidi, p.targetBidi)
	}()
	go func() {
		proxyErr <- p.proxyThread(p.targetBidi, p.originBidi)
	}()

	select {
	case err := <-proxyErr:
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, ErrProxyClosed) ||
			errors.Is(err, io.EOF) {
			return nil
		}

		// Stream was canceled by the other end.
		var streamErr *quic.StreamError
		if errors.As(err, &streamErr) {
			return nil
		}

		return err
	case <-p.ctx.Done():
		return nil
	}
}
