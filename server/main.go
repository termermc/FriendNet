package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"friendnet.org/server/cert"
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

	authStore, err := NewAuthStore(cfg)
	if err != nil {
		log.Fatalf("failed to load rooms: %v", err)
	}
	registry := NewClientRegistry()

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
	for _, listenAddr := range cfg.Listen {
		listener, err := listenQUIC(listenAddr, tlsCfg)
		if err != nil {
			log.Fatalf("failed to listen on %s: %v", listenAddr, err)
		}

		server := protocol.NewProtoServer(listener)
		server.AuthHandler = authStore.HandlerWithRegistry(registry)

		go acceptLoop(ctx, listenAddr, server, registry)
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

func listenQUIC(listenAddr string, tlsCfg *tls.Config) (*quic.Listener, error) {
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

func acceptLoop(ctx context.Context, listenAddr string, server *protocol.ProtoServer, registry *ClientRegistry) {
	for {
		client, err := server.Accept(ctx)
		if err != nil {
			log.Printf("accept error on %s: %v", listenAddr, err)
			continue
		}

		client.OnPing = func(_ context.Context, _ *protocol.ProtoServerClient, bidi protocol.ProtoBidi, _ *pb.MsgPing) error {
			return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{
				SentTs: time.Now().UnixMilli(),
			})
		}
		client.OnGetDirFiles = proxyDirFiles(registry)
		client.OnGetFileMeta = proxyFileMeta(registry)
		client.OnGetFile = proxyFile(registry)
		client.OnGetOnlineUsers = onlineUsersHandler(registry)

		clientCtx, cancel := context.WithCancel(ctx)
		go func() {
			defer cancel()
			defer registry.Unregister(client)
			if err := client.Listen(clientCtx, func(err error) {
				log.Printf("client listener error: %v", err)
			}); err != nil {
				log.Printf("client listen failed: %v", err)
			}
		}()
		go serverPingLoop(clientCtx, client)
	}
}

func serverPingLoop(ctx context.Context, client *protocol.ProtoServerClient) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := client.Ping(); err != nil {
				log.Printf("ping error: %v", err)
				return
			}
		}
	}
}

func onlineUsersHandler(registry *ClientRegistry) protocol.ServerGetOnlineUsersHandler {
	return func(_ context.Context, client *protocol.ProtoServerClient, bidi protocol.ProtoBidi, _ *pb.MsgGetOnlineUsers) error {
		info, ok := registry.Info(client)
		if !ok {
			message := "missing client info"
			return bidi.Write(pb.MsgType_MSG_TYPE_ERROR, &pb.MsgError{
				Type:    pb.ErrType_ERR_TYPE_INTERNAL,
				Message: &message,
			})
		}

		users := registry.ListUsers(info.Room)
		const chunkSize = 100
		for start := 0; start < len(users); start += chunkSize {
			end := start + chunkSize
			if end > len(users) {
				end = len(users)
			}
			if err := bidi.Write(pb.MsgType_MSG_TYPE_ONLINE_USERS, &pb.MsgOnlineUsers{Users: users[start:end]}); err != nil {
				return err
			}
		}
		return nil
	}
}
