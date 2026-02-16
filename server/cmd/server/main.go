package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"friendnet.org/protocol"
	"friendnet.org/server"
	"friendnet.org/server/cert"
	"friendnet.org/server/config"
	"friendnet.org/server/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	configPath := flag.String("config", "server.json", "path to server config JSON")
	flag.Parse()

	cfg, err := config.LoadOrCreate(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	keyPair, err := cert.ReadOrCreatePem(cfg.PemPath, cert.ServerCommonName)
	if err != nil {
		logger.Error("failed to load PEM", "err", err)
		os.Exit(1)
	}

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{keyPair},
		NextProtos:   []string{protocol.AlpnProtoName},
	}

	storageInst, err := storage.NewStorage(cfg.DbPath)
	if err != nil {
		logger.Error("failed to create storage", "err", err)
		os.Exit(1)
	}
	defer func() {
		_ = storageInst.Close()
	}()

	srv, err := server.NewServer(logger, storageInst)
	if err != nil {
		logger.Error("failed to create server", "err", err)
		os.Exit(1)
	}
	defer func() {
		_ = srv.Close()
	}()

	// Create RPC servers.
	rpcs := make([]*server.RpcServer, 0, len(cfg.Rpc.Interfaces))
	for _, iface := range cfg.Rpc.Interfaces {
		rpcSrv, err := server.NewRpcServer(logger, iface, srv)
		if err != nil {
			logger.Error(
				"failed to create RPC server",
				"address", iface.Address,
				"err", err,
			)
			os.Exit(1)
		}
		rpcs = append(rpcs, rpcSrv)
	}

	// Close server on SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, closing server")

		var wg sync.WaitGroup
		for _, rpc := range rpcs {
			wg.Go(func() {
				_ = rpc.Close()
			})
		}
		wg.Wait()
		_ = srv.Close()
	}()

	listenErrChan := make(chan error, len(cfg.Listen)+len(cfg.Rpc.Interfaces))

	for _, listenAddr := range cfg.Listen {
		go func() {
			listenErr := srv.Listen(listenAddr, tlsCfg)
			if listenErr != nil {
				logger.Error("failed to listen",
					"addr", listenAddr,
					"err", listenErr,
				)
			}
			listenErrChan <- listenErr
		}()
		logger.Info("server listening",
			"addr", listenAddr,
		)
	}

	for _, rpc := range rpcs {
		go func() {
			listenErr := rpc.Serve()
			if listenErr != nil {
				logger.Error("RPC server ended with error",
					"addr", rpc.Addr,
					"err", listenErr,
				)
			}
			listenErrChan <- listenErr
		}()
		logger.Info("RPC listening",
			"addr", rpc.Addr,
		)
	}

	endErr := <-listenErrChan
	if endErr == nil {
		logger.Info("server closed")
	} else {
		logger.Error("server ended with error", "err", endErr)
	}
}
