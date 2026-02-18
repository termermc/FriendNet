package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"friendnet.org/client"
	"friendnet.org/client/cert"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	rpcBearerToken := "abc123"

	// TODO Bearer token stored in DB
	// TODO Flag to reset bearer token

	var dataDir string
	var rpcAddr string

	flag.StringVar(&dataDir, "datadir", "", "path to server config JSON")
	flag.StringVar(&rpcAddr, "rpcaddr", "http://127.0.0.1:20039", "RPC server address")
	flag.Parse()

	if dataDir == "" {
		var err error
		dataDir, err = os.UserConfigDir()
		if err != nil {
			panic(fmt.Errorf(`failed to resolve user data directory: %w`, err))
		}
	}

	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		panic(fmt.Errorf(`failed to resolve absolute path for data directory %q: %w`, dataDir, err))
	}

	// Try to create data dir.
	err = os.MkdirAll(dataDir, 0755)
	if err != nil {
		panic(fmt.Errorf(`failed to create data directory: %w`, err))
	}

	dbDir := filepath.Join(dataDir, "client.db")

	store, err := storage.NewStorage(dbDir)
	if err != nil {
		panic(fmt.Errorf(`failed to create storage: %w`, err))
	}

	certStore := cert.NewSqliteStore(store.Db)

	multi, err := client.NewMultiClient(
		logger,
		store,
		certStore,
	)
	if err != nil {
		panic(fmt.Errorf(`failed to create multi client: %w`, err))
	}

	rpc, err := common.NewRpcServer(
		logger,
		common.RpcServerConfig{
			Address:        rpcAddr,
			AllowedMethods: []string{"*"},
			BearerToken:    rpcBearerToken,
		},
		client.NewRpcServer(multi),
		func(impl *client.RpcServer, options ...connect.HandlerOption) (string, http.Handler) {
			return clientrpcv1connect.NewClientRpcServiceHandler(impl, options...)
		},
	)
	if err != nil {
		_ = multi.Close()
		panic(fmt.Errorf(`failed to create RPC server: %w`, err))
	}

	logger.Info(`RPC server listening`,
		"addr", rpcAddr,
		"bearerToken", rpcBearerToken,
	)
	if err = rpc.Serve(); err != nil {
		panic(fmt.Errorf(`RPC server ended with error: %w`, err))
	}

	_ = rpc.Close()
	_ = multi.Close()
	_ = store.Close()
}
