package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/common"
	"friendnet.org/common/machine"
	"friendnet.org/common/password"
	"friendnet.org/common/webserver"
	"friendnet.org/protocol"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"friendnet.org/rpcclient"
	"friendnet.org/server"
	"friendnet.org/server/cert"
	"friendnet.org/server/config"
	"friendnet.org/server/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	var configPath string
	var noCli bool
	flag.StringVar(&configPath, "config", "server.json", "path to server config JSON")
	flag.BoolVar(&noCli, "nocli", false, "disable CLI")
	flag.Parse()

	cfg, err := config.LoadOrCreate(configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Check for insecure RPC interfaces that have wildcard permissions.
	for _, iface := range cfg.Rpc.Interfaces {
		if iface.BearerToken == "" && slices.Contains(iface.AllowedMethods, "*") {
			addr, _ := url.Parse(iface.Address)
			if addr.Scheme == "unix" {
				// UNIX sockets are exempt from warning.
				continue
			}

			logger.Warn("RPC interface has wildcard permissions but does not require a bearer token! THIS IS DANGEROUS!",
				"address", iface.Address,
			)
		}
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
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		go func() {
			_ = storageInst.Close()
			cancel()
		}()
		<-timeoutCtx.Done()
	}()

	// Probe for connection method support.
	connMethodSupport, err := machine.ProbeConnMethodSupport()
	if err != nil {
		logger.Warn("failed to probe for connection method support, support list will be incomplete",
			"err", err,
		)
	}

	// Server-wide password requirements.
	passReqs := password.NewRequirements(
		password.WithMinLen(8),
		password.WithMaxLen(64),
		password.WithCannotContainUsername(),
	)

	srv, err := server.NewServer(
		logger,
		storageInst,
		connMethodSupport,
		passReqs,
	)
	if err != nil {
		logger.Error("failed to create server", "err", err)
		os.Exit(1)
	}
	defer func() {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		go func() {
			_ = srv.Close()
			cancel()
		}()
		<-timeoutCtx.Done()
	}()

	// Create web server use for serving RPC interfaces.
	var rpcCert tls.Certificate
	if cfg.Rpc.HttpsFullChainCertPath != "" {
		rpcCert, err = cert.ReadFullChainPem(cfg.Rpc.HttpsFullChainCertPath)
		if err != nil {
			logger.Error("failed to read HTTPS certificate for RPC interfaces",
				"path", cfg.Rpc.HttpsFullChainCertPath,
				"err", err,
			)
			os.Exit(1)
		}
	} else {
		rpcCert = keyPair
	}
	webServer := webserver.NewWebServer(logger, webserver.WithHttpsSupport(rpcCert))

	// Create RPC servers.
	rpcServer := server.NewRpcServer(srv)
	rpcs := make([]*common.RpcServer[*server.RpcServer], 0, len(cfg.Rpc.Interfaces))
	for _, iface := range cfg.Rpc.Interfaces {
		rpcSrv, err := common.NewRpcServer(
			logger,
			webServer,
			iface,
			rpcServer,
			func(impl *server.RpcServer, options ...connect.HandlerOption) (string, http.Handler) {
				return serverrpcv1connect.NewServerRpcServiceHandler(impl, options...)
			},
		)
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

	if !noCli {
		go func() {
			localRpcToken := common.RandomB64UrlStr(32)
			expectAuthz := fmt.Sprintf("Bearer %s", localRpcToken)

			mux := http.NewServeMux()
			path, hdlr := serverrpcv1connect.NewServerRpcServiceHandler(rpcServer)
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != expectAuthz {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				hdlr.ServeHTTP(w, r)
			})

			clientHeaders := make(http.Header)
			clientHeaders.Set("Authorization", expectAuthz)
			localServer := httptest.NewServer(mux)
			cli := rpcclient.NewCli(
				serverrpcv1connect.NewServerRpcServiceClient(
					localServer.Client(),
					localServer.URL,
					connect.WithGRPCWeb(),
				),
				rpcclient.WithHeaders(clientHeaders),
				rpcclient.WithWelcomeMsg("Welcome to the FriendNet server CLI."),
			)
			cli.Run()
			stop()
		}()
	}

	go func() {
		<-ctx.Done()
		logger.Info("closing server")

		_ = webServer.Close()

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
		logger.Info("RPC listening",
			"addr", rpc.Addr,
		)
	}

	go func() {
		serveErr := webServer.Serve()
		if serveErr != nil {
			if errors.Is(serveErr, http.ErrServerClosed) {
				listenErrChan <- nil
				return
			}

			listenErrChan <- serveErr
		}
	}()

	endErr := <-listenErrChan

	if endErr == nil {
		logger.Info("server closed")
	} else {
		logger.Error("server ended with error", "err", endErr)
	}
}
