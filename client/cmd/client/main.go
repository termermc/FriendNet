package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/client"
	"friendnet.org/client/cert"
	"friendnet.org/client/clog"
	"friendnet.org/client/direct"
	"friendnet.org/client/event"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/common/machine"
	"friendnet.org/mkcert"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
	"friendnet.org/webui"
	"github.com/pkg/browser"
)

const lockFilename = "client-lock.json"

type LockData struct {
	Ts      int64  `json:"ts"`
	RpcAddr string `json:"rpc_addr"`
}

type Locker struct {
	lockDir string
}

func (l *Locker) CheckLock() *LockData {
	filePath := filepath.Join(l.lockDir, lockFilename)
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var data LockData
	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		_ = os.Remove(filePath)
		return nil
	}

	rpcUrl, err := url.Parse(data.RpcAddr)
	if err != nil {
		_ = os.Remove(filePath)
		return nil
	}

	// See if we can dial the RPC address in the lock.
	conn, err := net.DialTimeout("tcp", rpcUrl.Host, 1*time.Second)
	if err != nil {
		// Failed to dial address; this is probably a stale lock.
		_ = os.Remove(filePath)
		return nil
	}

	// We can dial the address. The client is truly locked.
	_ = conn.Close()
	return &data
}

func (l *Locker) Lock(rpcAddr string) error {
	filePath := filepath.Join(l.lockDir, lockFilename)
	data := LockData{
		Ts:      time.Now().UnixMilli(),
		RpcAddr: rpcAddr,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal lock data: %w", err)
	}
	err = os.WriteFile(filePath, jsonData, 0600)
	if err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}
	return nil
}

func (l *Locker) Unlock() {
	filePath := filepath.Join(l.lockDir, lockFilename)
	_ = os.Remove(filePath)
}

func main() {
	runId := time.Now().UnixMilli()

	// TODO Bearer token stored in DB
	// TODO Flag to reset bearer token
	rpcBearerToken := "abc123"

	var dataDir string
	var rpcAddr string
	var fileAddr string
	var uiAddr string
	var headless bool
	var noBrowser bool
	var noLock bool
	var installCa bool
	var uninstallCa bool

	flag.StringVar(&dataDir, "datadir", "", "path to server config JSON")
	flag.StringVar(&rpcAddr, "rpcaddr", "https://localhost:20039", "RPC server address")
	flag.StringVar(&fileAddr, "fileaddr", "https://localhost:20040", "File server address")
	flag.StringVar(&uiAddr, "uiaddr", "https://localhost:20041", "Web UI server address")
	flag.BoolVar(&noBrowser, "nobrowser", false, "do not open web UI in browser")
	flag.BoolVar(&noLock, "nolock", false, "do not use a lock to prevent multiple instances of the client from running")
	flag.BoolVar(&installCa, "installca", false, "if set, tries to install the client's root CA for HTTPS on the web UI")
	flag.BoolVar(&uninstallCa, "uninstallca", false, "if set, tries to uninstall the client's root CA")

	// Prevent headless mode on Windows.
	// It just causes the process to go to the background and not stay in the terminal.
	if runtime.GOOS != "windows" {
		flag.BoolVar(&headless, "headless", false, "run client in headless mode (RPC-only, no web UI, no locking, no GUI or browser functionality)")
	}

	flag.Parse()

	if headless {
		noBrowser = true
		noLock = true
	}

	println(headless)
	println(noBrowser)
	println(noLock)

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

	rpcUrl, err := url.Parse(rpcAddr)
	if err != nil {
		panic(fmt.Errorf(`failed to parse RPC server address %q: %w`, rpcAddr, err))
	}

	fileUrl, err := url.Parse(fileAddr)
	if err != nil {
		panic(fmt.Errorf(`failed to parse file server address %q: %w`, fileAddr, err))
	}
	if fileUrl.Scheme != "https" {
		panic(fmt.Errorf(`file server address must start with "https://" scheme`))
	}

	uiUrl, err := url.Parse(uiAddr)
	if err != nil {
		panic(fmt.Errorf(`failed to parse web UI server address %q: %w`, uiAddr, err))
	}
	if uiUrl.Scheme != "https" {
		panic(fmt.Errorf(`web UI server address must start with "https://" scheme`))
	}

	if !noLock {
		locker := &Locker{
			lockDir: dataDir,
		}
		lockData := locker.CheckLock()
		if lockData != nil {
			println("Client is already running")

			if !noBrowser {
				// Try to open web UI in browser.
				_ = browser.OpenURL(uiUrl.String())
			}

			return
		}

		err = locker.Lock(rpcAddr)
		if err != nil {
			panic(fmt.Errorf(`failed to lock client: %w`, err))
		}
		defer locker.Unlock()
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

	mc, err := mkcert.NewMkCert(dataDir)
	if err != nil {
		logger.Error(`failed to initialize mkcert`, "err", err)
		os.Exit(1)
	}

	if installCa {
		if err = mc.Install(); err != nil {
			logger.Error(`failed to install client root CA`, "err", err)
			os.Exit(1)
		}
		return
	}
	if uninstallCa {
		if err = mc.Uninstall(); err != nil {
			logger.Error(`failed to uninstall client root CA`, "err", err)
			os.Exit(1)
		}
		return
	}

	if !headless && !mc.CheckPlatform() {
		installed := false
		if runtime.GOOS == "windows" {
			InfoBox("FriendNet Client", "It looks like this is your first time running the FriendNet client.\n\nThe web UI requires HTTPS and a custom certificate, so that will be installed now. If it is not installed, the web UI will not work in your browser.\n\nYou may need to restart your browser afterward.")
			if err = mc.Install(); err != nil {
				logger.Error(`failed to install client root CA`, "err", err)
				InfoBox("Error", "Failed to install FriendNet client root CA. Please try again or install it manually by running the client with the -installca option.\n\nError: "+err.Error())
				os.Exit(1)
			}
			installed = true
		}

		if !installed {
			logger.Warn("The FriendNet client root CA is not installed on your system. You should install it by running the client with the -installca option.")
		}
	}

	certStore := cert.NewSqliteStore(store.Db)

	directCfg, err := direct.ConfigFromSettings(context.Background(), store)
	if err != nil {
		logger.Error(`failed to load direct configuration`, "err", err)
		os.Exit(1)
	}
	directMgr, err := direct.NewManager(logger, directCfg)
	if err != nil {
		logger.Error(`failed to create direct manager`, "err", err)
		os.Exit(1)
	}

	// Probe for connection method support.
	connMethodSupport, err := machine.ProbeConnMethodSupport()
	if err != nil {
		logger.Warn("failed to probe for connection method support, support list will be incomplete",
			"err", err,
		)
	}

	eventBus := event.NewBus()

	multi, err := client.NewMultiClient(
		logger,
		store,
		certStore,
		connMethodSupport,
		directMgr,
		eventBus,
	)
	if err != nil {
		panic(fmt.Errorf(`failed to create multi client: %w`, err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpsCertPem, httpsKeyPem, err := store.GetClientHttpsCert(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpsCertPem, httpsKeyPem, err = mc.GenCert([]string{uiUrl.Hostname(), fileUrl.Hostname()})
			if err != nil {
				panic(fmt.Errorf(`failed to generate HTTPS certificate: %w`, err))
			}
			err = store.SetClientHttpsCert(ctx, httpsCertPem, httpsKeyPem)
			if err != nil {
				panic(fmt.Errorf(`failed to store HTTPS certificate: %w`, err))
			}
		} else {
			panic(fmt.Errorf(`failed to retrieve HTTPS certificate: %w`, err))
		}
	}

	httpsKeyPair, err := tls.X509KeyPair(httpsCertPem, httpsKeyPem)
	if err != nil {
		panic(fmt.Errorf(`failed to parse HTTPS certificate key pair: %w`, err))
	}
	httpsTlsCfg := &tls.Config{
		Certificates: []tls.Certificate{httpsKeyPair},
	}

	var rpcTls *tls.Config
	if rpcUrl.Scheme == "https" {
		rpcTls = httpsTlsCfg
	}

	rpc, err := common.NewRpcServer(
		logger,
		common.RpcServerConfig{
			Address:             rpcAddr,
			AllowedMethods:      []string{"*"},
			BearerToken:         rpcBearerToken,
			CorsAllowAllOrigins: true,
		},
		client.NewRpcServer(
			logHandler,
			multi,
			eventBus,
			fileAddr,
			stop,
		),
		func(impl *client.RpcServer, options ...connect.HandlerOption) (string, http.Handler) {
			return clientrpcv1connect.NewClientRpcServiceHandler(impl, options...)
		},
		rpcTls,
	)
	if err != nil {
		_ = multi.Close()
		panic(fmt.Errorf(`failed to create RPC server: %w`, err))
	}

	httpProto := &http.Protocols{}
	httpProto.SetHTTP2(true)
	httpProto.SetHTTP1(true)
	fileServer := http.Server{
		Addr:      fileUrl.Host,
		Handler:   client.NewFileServer(logger, multi),
		TLSConfig: httpsTlsCfg,
		Protocols: httpProto,
	}
	uiServer := http.Server{
		Addr:      uiUrl.Host,
		Handler:   webui.Handler{},
		TLSConfig: httpsTlsCfg,
		Protocols: httpProto,
	}

	// Close client on SIGTERM.
	var shutdownWg sync.WaitGroup
	defer stop()
	shutdownWg.Go(func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, closing client")

		doWithTimeout := func(timeout time.Duration, fn func(ctx context.Context)) {
			timeoutCtx, ctxCancel := context.WithTimeout(context.Background(), timeout)
			go func() {
				fn(timeoutCtx)
				ctxCancel()
			}()
			<-timeoutCtx.Done()
		}

		doWithTimeout(1*time.Second, func(ctx context.Context) {
			_ = uiServer.Shutdown(ctx)
			_ = fileServer.Shutdown(ctx)
		})
		doWithTimeout(1*time.Second, func(_ context.Context) {
			_ = rpc.Close()
		})
		doWithTimeout(5*time.Second, func(_ context.Context) {
			_ = multi.Close()
		})
		doWithTimeout(5*time.Second, func(_ context.Context) {
			_ = logHandler.Close()
		})
		doWithTimeout(5*time.Second, func(_ context.Context) {
			_ = store.Close()
		})
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
		listenErr := fileServer.ListenAndServeTLS("", "")
		if listenErr != nil {
			if errors.Is(listenErr, http.ErrServerClosed) {
				return
			}
			panic(fmt.Errorf(`file server ended with error: %w`, listenErr))
		}
	})
	if !headless {
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

			listenErr := uiServer.ListenAndServeTLS("", "")
			if listenErr != nil {
				if errors.Is(listenErr, http.ErrServerClosed) {
					return
				}
				panic(fmt.Errorf(`web UI server ended with error: %w`, listenErr))
			}
		})
	}

	wg.Wait()

	stop()

	shutdownWg.Wait()
}
