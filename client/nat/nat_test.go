package nat

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net"
	"testing"
	"time"

	"friendnet.org/common"
	"github.com/quic-go/quic-go"
)

func mkTls() (*tls.Config, error) {
	pem, err := common.GenSelfSignedPem("test", false)
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(pem, pem)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{"test"},
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
	}, nil
}

// TestQuicListenDialSameAddr tests dialing and listening on same UDP address.
func TestQuicListenDialSameAddr(t *testing.T) {
	tlsCfg, err := mkTls()
	if err != nil {
		t.Fatal(err)
	}

	portA := rand.IntN(65535-1024) + 1024
	portB := rand.IntN(65535-1024) + 1024

	addrA := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: portA,
	}
	addrB := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: portB,
	}

	udpA, err := net.ListenUDP("udp", addrA)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = udpA.Close()
	}()
	udpB, err := net.ListenUDP("udp", addrB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = udpB.Close()
	}()

	trA := &quic.Transport{
		Conn: udpA,
	}
	trB := &quic.Transport{
		Conn: udpB,
	}

	listenChan := make(chan error, 1)
	go func() {
		l, err := trA.Listen(tlsCfg, nil)
		if err != nil {
			listenChan <- fmt.Errorf(`listener A failed: %w`, err)
			return
		}
		listenChan <- nil

		for {
			conn, err := l.Accept(t.Context())
			if err != nil {
				return
			}

			println("A got conn from " + conn.RemoteAddr().String())
		}
	}()
	go func() {
		l, err := trB.Listen(tlsCfg, nil)
		if err != nil {
			listenChan <- fmt.Errorf(`listener B failed: %w`, err)
			return
		}
		listenChan <- nil

		for {
			conn, err := l.Accept(t.Context())
			if err != nil {
				return
			}

			println("B got conn from " + conn.RemoteAddr().String())
		}
	}()

	// Wait for listeners to start.
	if err = <-listenChan; err != nil {
		t.Fatal(err)
	}
	if err = <-listenChan; err != nil {
		t.Fatal(err)
	}

	dialChan := make(chan error, 1)

	go func() {
		_, err := trA.Dial(context.Background(), addrB, tlsCfg, nil)
		if err != nil {
			dialChan <- fmt.Errorf(`dial A to B failed: %w`, err)
			return
		}

		dialChan <- nil
	}()
	go func() {
		_, err := trB.Dial(context.Background(), addrA, tlsCfg, nil)
		if err != nil {
			dialChan <- fmt.Errorf(`dial B to A failed: %w`, err)
			return
		}

		dialChan <- nil
	}()

	if err = <-dialChan; err != nil {
		t.Fatal(err)
	}
	if err = <-dialChan; err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)
}
