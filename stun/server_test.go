package stun

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"friendnet.org/common"
	"github.com/quic-go/quic-go"
)

func runServer(sock *net.UDPConn) error {
	return RunServer(
		context.Background(),
		func(_ context.Context, b []byte) (int, netip.AddrPort, error) {
			return sock.ReadFromUDPAddrPort(b)
		},
		sock.WriteToUDPAddrPort,
	)
}

func testRunServer(t *testing.T, network string) netip.AddrPort {
	clientPort := uint16(19313)

	serverSock := mkSockPortNet(19312, network)
	clientSock := mkSockPortNet(clientPort, network)
	defer func() {
		_ = serverSock.Close()
		_ = clientSock.Close()
	}()

	go func() {
		err := runServer(serverSock)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			t.Error(err)
			return
		}
	}()

	selfAddrPort, err := GetPublicAddrPort(clientSock, serverSock.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}

	if selfAddrPort.Port() != clientPort {
		t.Fatalf("expected port %d, got %d", clientPort, selfAddrPort.Port())
	}

	return selfAddrPort
}

func TestRunServer4(t *testing.T) {
	selfAddrPort := testRunServer(t, "udp4")
	if selfAddrPort.Addr().String() != "127.0.0.1" {
		t.Fatalf("expected IPv4 address 127.0.0.1, got %s", selfAddrPort.Addr().String())
	}
}
func TestRunServer6(t *testing.T) {
	selfAddrPort := testRunServer(t, "udp6")
	if selfAddrPort.Addr().String() != "::1" {
		t.Fatalf("expected IPv6 address ::1, got %s", selfAddrPort.Addr().String())
	}
}

func TestQuicMultiplex(t *testing.T) {
	clientPort := uint16(19313)

	serverSock := mkSockPortNet(19312, "udp4")
	clientSock := mkSockPortNet(clientPort, "udp4")
	defer func() {
		_ = serverSock.Close()
		_ = clientSock.Close()
	}()

	tr := &quic.Transport{
		Conn: serverSock,
	}
	certPem, err := common.GenSelfSignedPem("localhost", false)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(certPem, certPem)
	if err != nil {
		t.Fatal(err)
	}

	qListener, err := tr.Listen(
		&tls.Config{
			Certificates: []tls.Certificate{cert},
		},
		&quic.Config{},
	)
	defer func() {
		_ = qListener.Close()
	}()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	defer serverCancel()
	go func() {
		err := RunServer(
			serverCtx,
			func(ctx context.Context, b []byte) (int, netip.AddrPort, error) {
				n, addr, err := tr.ReadNonQUICPacket(ctx, b)
				if err != nil {
					return 0, netip.AddrPort{}, err
				}

				return n, netip.MustParseAddrPort(addr.String()), nil
			},
			serverSock.WriteToUDPAddrPort,
		)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
				return
			}

			t.Error(err)
			return
		}
	}()

	// Sometimes a second try is needed with the multiplexed connections.
	// I don't know why, maybe the packet got lost?
	// In a normal setup, the client will query multiple times, so we'll do it here.
	_ = clientSock.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	selfAddrPort, err := GetPublicAddrPort(clientSock, serverSock.LocalAddr().String())
	if err != nil {
		if opErr, ok := errors.AsType[*net.OpError](err); ok && opErr.Timeout() {
			// Try again.
			t.Log("trying again")
			_ = clientSock.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			selfAddrPort, err = GetPublicAddrPort(clientSock, serverSock.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatal(err)
		}
	}

	if selfAddrPort.Port() != clientPort {
		t.Fatalf("expected port %d, got %d", clientPort, selfAddrPort.Port())
	}

	if selfAddrPort.Addr().String() != "127.0.0.1" {
		t.Fatalf("expected IPv4 address 127.0.0.1, got %s", selfAddrPort.Addr().String())
	}
}
