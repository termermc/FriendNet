package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"os"

	"friendnet.org/protocol"
	"friendnet.org/server/cert"
	"friendnet.org/server/room"
	"friendnet.org/server/storage"
	"github.com/quic-go/quic-go"
)

const selfSignedPem = "server.pem"

func main() {
	configPath := flag.String("config", "server.json", "path to server config JSON")
	pemPath := flag.String("pem", selfSignedPem, "path to PEM file")
	flag.Parse()

	cfg, err := LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	pemFile, err := readOrCreatePem(*pemPath)
	if err != nil {
		log.Fatalf("failed to load PEM: %v", err)
	}

	keyPair, err := tls.X509KeyPair(pemFile, pemFile)
	if err != nil {
		log.Fatalf("failed to parse key pair: %v", err)
	}

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{keyPair},
		NextProtos:   []string{protocol.AlpnProtoName},
	}

	ctx := context.Background()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	//goland:noinspection GoResourceLeak
	storageInst, err := storage.NewStorage(cfg.DbPath)
	if err != nil {
		log.Fatalf("failed to create storage: %v", err)
	}

	//goland:noinspection GoResourceLeak
	roomMgr, err := room.NewManager(ctx, logger, storageInst)

	lobby := NewLobby(logger, storageInst, roomMgr, DefaultLobbyTimeout, protocol.CurrentProtocolVersion)

	for _, listenAddr := range cfg.Listen {
		//goland:noinspection GoResourceLeak
		listener, err := listenQuic(listenAddr, tlsCfg)
		if err != nil {
			log.Fatalf("failed to listen on %s: %v", listenAddr, err)
		}

		go acceptLoop(ctx, listener, listenAddr, lobby)
		log.Printf("listening on %s", listenAddr)
	}

	select {}
}

func readOrCreatePem(path string) ([]byte, error) {
	pemFile, err := os.ReadFile(path)
	if err == nil {
		return pemFile, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	pemFile, err = cert.GenSelfSignedPem("friendnet-server")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, pemFile, 0o600); err != nil {
		return nil, err
	}
	return pemFile, nil
}

func listenQuic(listenAddr string, tlsCfg *tls.Config) (*quic.Listener, error) {
	addrPort, err := netip.ParseAddrPort(listenAddr)
	if err != nil {
		return nil, fmt.Errorf(`failed to parse listen address %q: %w`, listenAddr, err)
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
	if err != nil {
		return nil, err
	}

	trans := quic.Transport{Conn: udpConn}
	return trans.Listen(tlsCfg, &quic.Config{
		KeepAlivePeriod: protocol.DefaultKeepAlivePeriod,
	})
}

func acceptLoop(
	ctx context.Context,
	listener *quic.Listener,
	listenAddr string,
	lobby *Lobby,
) {
	for {
		rawConn, err := listener.Accept(ctx)
		if err != nil {
			log.Printf("accept error on %s: %v", listenAddr, err)
			continue
		}

		// Wrap connection.
		conn := protocol.ToProtoConn(rawConn)

		// Pass it to the lobby.
		lobby.Onboard(conn)
	}
}
