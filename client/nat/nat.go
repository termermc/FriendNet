package nat

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/quic-go/quic-go"
)

func TryTraverse(
	ctx context.Context,
	listenAddr string,
	peerAddr string,
	token string,
	tlsConf *tls.Config,
	quicConf *quic.Config,
) (*quic.Conn, error) {
	listenAddrPort, err := netip.ParseAddrPort(listenAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to parse listen address %q: %w`, listenAddr, err)
	}
	peerAddrPort, err := netip.ParseAddrPort(peerAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to parse peer address %q: %w`, peerAddr, err)
	}
	peerUdpAddr := &net.UDPAddr{
		IP:   peerAddrPort.Addr().AsSlice(),
		Port: int(peerAddrPort.Port()),
	}

	udp, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   listenAddrPort.Addr().AsSlice(),
		Port: int(listenAddrPort.Port()),
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to listen on UDP %q: %w`, listenAddr, err)
	}

	qListener, err := quic.Listen(udp, tlsConf, quicConf)
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

			conn, err := quic.Dial(ctx, udp, peerUdpAddr, tlsConf, quicConf)
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
