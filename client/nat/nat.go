package nat

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// TryTraverse attempts NAT traversal by sending UDP packets to a peer while listening or dialing.
// If isServerSide is true, it will listen for an incoming connection, otherwise it will dial.
// TODO Could I revert to the old design where they both listen and dial at the same time?
// I could return a channel that can return up to 2 quic.Conn instances, closing the channel when all attempts have
// finished, whether successfully or not. The caller could choose to handle the conns in a goroutine and manage its own
// timeout for handling them.
// This would be advantageous because some weird NATs allow incoming traffic. Still, I'm not sure how many cases that
// will work for, because most system firewalls will stop this behavior from working anyway. I'm leaning toward keeping
// things the way they are.
// TODO Also, run a persistent QUIC listenener as a NAT direct server, and expose its quic.Transport so that it can be
// used for discovery. This will simplify design, and prevent needing to spin up a new server for each hole punch
// attempt.
func TryTraverse(
	ctx context.Context,
	listenAddr string,
	peerAddr string,
	isServerSide bool,
	tlsConf *tls.Config,
	quicConf *quic.Config,
) (*quic.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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

	// Send out UDP packets to hole punch.
	go func() {
		tokenBytes := make([]byte, 100)
		_, _ = rand.Read(tokenBytes)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = udp.WriteToUDP(tokenBytes, peerUdpAddr)
			}
		}
	}()

	var conn *quic.Conn

	if isServerSide {
		qListener, err := tr.Listen(tlsConf, quicConf)
		if err != nil {
			return nil, fmt.Errorf(`failed to listen QUIC on existing UDP listener on %q: %w`, listenAddr, err)
		}

		conn, err = qListener.Accept(ctx)
		if err != nil {
			return nil, fmt.Errorf(`failed to accept incoming QUIC connection on %q: %w`, listenAddr, err)
		}
	} else {
		conn, err = tr.Dial(ctx, peerUdpAddr, tlsConf, quicConf)
		if err != nil {
			return nil, fmt.Errorf(`failed to dial QUIC %q: %w`, peerAddr, err)
		}
	}

	return conn, nil
}
