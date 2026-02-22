package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/client"
	"friendnet.org/client/cert"
	"friendnet.org/client/clog"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
	"friendnet.org/webui"
	"github.com/pkg/browser"
)

func main() {
	runId := time.Now().UnixMilli()

	// TODO Bearer token stored in DB
	// TODO Flag to reset bearer token
	rpcBearerToken := "abc123"

	var dataDir string
	var rpcAddr string
	var fileAddr string
	var uiAddr string
	var noBrowser bool

	flag.StringVar(&dataDir, "datadir", "", "path to server config JSON")
	flag.StringVar(&rpcAddr, "rpcaddr", "http://127.0.0.1:20039", "RPC server address")
	flag.StringVar(&fileAddr, "fileaddr", "http://127.0.0.1:20040", "File server address")
	flag.StringVar(&uiAddr, "uiaddr", "http://127.0.0.1:20041", "Web UI server address")
	flag.BoolVar(&noBrowser, "nobrowser", false, "do not open web UI in browser")
	flag.Parse()

	if dataDir == "" {
		var err error
		dataDir, err = GetDataDir()
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

	if !strings.HasPrefix(fileAddr, "http://") {
		panic(fmt.Errorf(`file server address must start with "http://" scheme`))
	}
	if !strings.HasPrefix(uiAddr, "http://") {
		panic(fmt.Errorf(`web UI server address must start with "http://" scheme`))
	}

	dbDir := filepath.Join(dataDir, "client.db")

	store, err := storage.NewStorage(dbDir)
	if err != nil {
		panic(fmt.Errorf(`failed to create storage: %w`, err))
	}

	// Create logger after storage is initialized, as it depends on migrations being run.
	logHandler := clog.NewHandler(
		store.Db,
		runId,
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	)
	logger := slog.New(logHandler)

	certStore := cert.NewSqliteStore(store.Db)

	multi, err := client.NewMultiClient(
		logger,
		store,
		certStore,
	)
	if err != nil {
		panic(fmt.Errorf(`failed to create multi client: %w`, err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	rpc, err := common.NewRpcServer(
		logger,
		common.RpcServerConfig{
			Address:             rpcAddr,
			AllowedMethods:      []string{"*"},
			BearerToken:         rpcBearerToken,
			CorsAllowAllOrigins: true,
		},
		client.NewRpcServer(
			multi,
			fileAddr,
			stop,
		),
		func(impl *client.RpcServer, options ...connect.HandlerOption) (string, http.Handler) {
			return clientrpcv1connect.NewClientRpcServiceHandler(impl, options...)
		},
	)
	if err != nil {
		_ = multi.Close()
		panic(fmt.Errorf(`failed to create RPC server: %w`, err))
	}

	fileServer := http.Server{
		Addr:    fileAddr[7:],
		Handler: client.NewFileServer(logger, multi),
	}
	uiServer := http.Server{
		Addr:    uiAddr[7:],
		Handler: webui.Handler{},
	}

	// Close client on SIGTERM.
	var shutdownWg sync.WaitGroup
	defer stop()
	shutdownWg.Go(func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, closing client")

		timeoutCtx, ctxCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer ctxCancel()

		_ = uiServer.Shutdown(timeoutCtx)
		_ = fileServer.Shutdown(timeoutCtx)
		_ = rpc.Close()
		_ = multi.Close()
		_ = logHandler.Close()
		_ = store.Close()
	})

	var wg sync.WaitGroup
	wg.Go(func() {
		logger.Info(`RPC server listening`,
			"addr", rpcAddr,
			"bearerToken", rpcBearerToken,
		)
		if listenErr := rpc.Serve(); listenErr != nil {
			panic(fmt.Errorf(`RPC server ended with error: %w`, listenErr))
		}
	})
	wg.Go(func() {
		logger.Info(`File server listening`, "addr", fileAddr)
		listenErr := fileServer.ListenAndServe()
		if listenErr != nil {
			if errors.Is(listenErr, http.ErrServerClosed) {
				return
			}
			panic(fmt.Errorf(`file server ended with error: %w`, listenErr))
		}
	})
	wg.Go(func() {
		uiUrl := fmt.Sprintf("%s?rpc=%s&token=%s", uiAddr, rpcAddr, rpcBearerToken)

		logger.Info(`Web UI server listening`,
			"addr", uiAddr,
			"url", uiUrl,
		)

		if !noBrowser {
			// Try to open URL in browser.
			_ = browser.OpenURL(uiUrl)
		}

		listenErr := uiServer.ListenAndServe()
		if listenErr != nil {
			if errors.Is(listenErr, http.ErrServerClosed) {
				return
			}
			panic(fmt.Errorf(`web UI server ended with error: %w`, listenErr))
		}
	})

	wg.Wait()

	stop()

	shutdownWg.Wait()
}
