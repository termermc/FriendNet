package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"os"

	"friendnet.org/protocol"
	"friendnet.org/server/cert"
	"github.com/quic-go/quic-go"
)

const selfSignedPem = "server.pem"

var listenAddrs = []string{
	"127.0.0.1:20038",
	"[::1]:20038",
}

func main() {
	// Read or create self-signed PEM for server.
	var pemFile []byte
	{
		var err error
		pemFile, err = os.ReadFile(selfSignedPem)
		if err != nil {
			if os.IsNotExist(err) {
				// Create PEM file.
				pemFile, err = cert.GenSelfSignedPem("friendnet-server")
				if err != nil {
					panic(err)
				}

				err = os.WriteFile(selfSignedPem, pemFile, 0o600)
				if err != nil {
					panic(err)
				}
			} else {
				panic(err)
			}
		}
	}

	// Create TLS config from PEM.
	keyPair, err := tls.X509KeyPair(pemFile, pemFile)
	if err != nil {
		panic(fmt.Errorf("failed to parse key pair: %w", err))
	}

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{keyPair},
		NextProtos: []string{
			protocol.AlpnProtoName,
		},
	}

	// Start listeners.
	for _, listenAddr := range listenAddrs {
		addrPort, err := netip.ParseAddrPort(listenAddr)
		if err != nil {
			panic(fmt.Errorf(`failed to parse listen address %q: %w`, listenAddr, err))
		}

		var udpConn *net.UDPConn
		addr := addrPort.Addr()
		if addr.Is6() {
			udpConn, err = net.ListenUDP("udp6", &net.UDPAddr{
				IP:   addr.AsSlice(),
				Port: int(addrPort.Port()),
			})
		} else {
			udpConn, err = net.ListenUDP("udp4", &net.UDPAddr{
				IP:   addr.AsSlice(),
				Port: int(addrPort.Port()),
			})
		}

		trans := quic.Transport{
			Conn: udpConn,
		}

		listener, err := trans.Listen(tlsCfg, &quic.Config{})

		_ = listener
		//for {
		//	conn, err := listener.Accept()
		//}
	}
}
