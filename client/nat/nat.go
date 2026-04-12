package nat

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// TryTraverse attempts NAT traversal by sending UDP packets to a peer while simultaneously listening and dialing.
func TryTraverse(
	ctx context.Context,
	listenAddr string,
	peerAddr string,
	token string,
	tlsConf *tls.Config,
	quicConf *quic.Config,
) (*quic.Conn, error) {
	listenUdpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to resolve listen address %q: %w`, listenAddr, err)
	}
	peerUdpAddr, err := net.ResolveUDPAddr("udp", peerAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to resolve peer address %q: %w`, peerAddr, err)
	}

	udp, err := net.ListenUDP("udp", listenUdpAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to listen on UDP %q: %w`, listenAddr, err)
	}
	tr := &quic.Transport{
		Conn: udp,
	}

	qListener, err := tr.Listen(tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf(`failed to listen QUIC on existing UDP listener on %q: %w`, listenAddr, err)
	}

	connChan := make(chan *quic.Conn)

	isRunning := true
	defer func() {
		isRunning = false
	}()

	// Listen for an incoming connection.
	go func() {
		for isRunning {
			conn, err := qListener.Accept(ctx)
			if err != nil {
				continue
			}

			// TODO Wait for incoming verification token.

			connChan <- conn
			return
		}
	}()

	// Try outgoing connections.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			if !isRunning {
				return
			}

			conn, err := tr.Dial(ctx, peerUdpAddr, tlsConf, quicConf)
			if err != nil {
				continue
			}

			// TODO Send out verification token and wait for ACK.

			connChan <- conn
			return
		}
	}()

	// Send out UDP packets to hole punch.
	go func() {
		tokenBytes := []byte(token)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if !isRunning {
				return
			}

			_, _ = udp.WriteToUDP(tokenBytes, peerUdpAddr)
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-connChan:
		return conn, nil
	}
}
