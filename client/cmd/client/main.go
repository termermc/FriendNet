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
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/client"
	"friendnet.org/client/appinfo"
	"friendnet.org/client/cert"
	"friendnet.org/client/clog"
	"friendnet.org/client/direct"
	"friendnet.org/client/event"
	"friendnet.org/client/fsys"
	"friendnet.org/client/fsys/multifs"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/common/machine"
	"friendnet.org/common/updater"
	"friendnet.org/common/webserver"
	"friendnet.org/mkcert"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
	"friendnet.org/webui"
	"github.com/pkg/browser"
	"golang.org/x/net/webdav"
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

	var dataDir string
	var webAddr string
	var davAddr string
	var headless bool
	var noBrowser bool
	var noLock bool
	var installCa bool
	var uninstallCa bool
	var resetToken bool
	var pprofFile string
	var rmCertHost string

	flag.StringVar(&dataDir, "datadir", "", "path to the client's data directory")
	flag.StringVar(&webAddr, "webaddr", "https://127.0.0.1:20042", "web UI and RPC address")
	flag.StringVar(&davAddr, "davaddr", "https://127.0.0.1:20043", "WebDAV server address")
	flag.BoolVar(&noBrowser, "nobrowser", false, "do not open web UI in browser")
	flag.BoolVar(&noLock, "nolock", false, "do not use a lock to prevent multiple instances of the client from running")
	flag.BoolVar(&installCa, "installca", false, "if set, tries to install the client's root CA for HTTPS on the web UI")
	flag.BoolVar(&uninstallCa, "uninstallca", false, "if set, tries to uninstall the client's root CA")
	flag.BoolVar(&resetToken, "resettoken", false, "if set, resets the bearer token for the RPC server")
	flag.StringVar(&pprofFile, "pproffile", "", "write CPU profile data in the pprof format to this file, e.g. \"cpu.pprof\"")
	flag.StringVar(&rmCertHost, "rmcerthost", "", "removes the specified host from the certificate store (like removing a host from SSH known_hosts)")

	// Prevent headless mode on Windows.
	// It just causes the process to go to the background and not stay in the terminal.
	if runtime.GOOS != "windows" {
		flag.BoolVar(&headless, "headless", false, "run client in headless mode (RPC-only, no web UI, no locking, no GUI or browser functionality)")
	}

	flag.Parse()

	var profilerFile *os.File
	if pprofFile != "" {
		var err error
		//goland:noinspection GoResourceLeak
		profilerFile, err = os.Create(pprofFile)
		if err != nil {
			panic(fmt.Errorf(`failed to create pprof file: %w`, err))
		}
		if err = pprof.StartCPUProfile(profilerFile); err != nil {
			_ = profilerFile.Close()
			panic(fmt.Errorf(`failed to start CPU profile: %w`, err))
		}
		println("Running profiler, writing data to " + pprofFile)
	}

	if headless {
		noBrowser = true
		noLock = true
	}

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

	webUrl, err := url.Parse(webAddr)
	if err != nil {
		panic(fmt.Errorf(`failed to parse web UI server address %q: %w`, webAddr, err))
	}

	_, err = url.Parse(davAddr)
	if err != nil {
		panic(fmt.Errorf(`failed to parse WebDAV server address %q: %w`, davAddr, err))
	}

	dbDir := filepath.Join(dataDir, "client.db")

	store, err := storage.NewStorage(dbDir)
	if err != nil {
		panic(fmt.Errorf(`failed to create storage: %w`, err))
	}

	certStore := cert.NewSqliteStore(store)

	if rmCertHost != "" {
		has, rmErr := certStore.DeleteDer(context.Background(), rmCertHost)
		if rmErr != nil {
			panic(rmErr)
		}

		if has {
			println("certificate removed for host")
		} else {
			println("no certificate found for host, nothing to remove")
		}
		return
	}

	// Create logger after storage is initialized, as it depends on migrations being run.
	logHandler := clog.NewHandler(
		store,
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

	// Get or set bearer token.
	var rpcBearerToken string
	const rpcTokenSetting = "rpc_bearer_token"
	{
		const byteLen = 32
		if resetToken {
			rpcBearerToken = common.RandomB64UrlStr(byteLen)
			err = store.PutSetting(context.Background(), rpcTokenSetting, rpcBearerToken)
		} else {
			rpcBearerToken, err = store.GetSettingOrPut(context.Background(), rpcTokenSetting, common.RandomB64UrlStr(byteLen))
		}
		if err != nil {
			logger.Error(`failed to get or set RPC bearer token`, "err", err)
			os.Exit(1)
		}
	}

	webUrlWithCreds := strings.ReplaceAll(fmt.Sprintf("%s?token=%s", webUrl.String(), rpcBearerToken), "127.0.0.1", "localhost")

	if !noLock {
		locker := &Locker{
			lockDir: dataDir,
		}
		lockData := locker.CheckLock()
		if lockData != nil {
			println("Client is already running")

			if !noBrowser {
				// Try to open web UI in browser.
				_ = browser.OpenURL(webUrlWithCreds)
			}

			_ = logHandler.Close()
			_ = store.Close()

			return
		}

		err = locker.Lock(webAddr)
		if err != nil {
			panic(fmt.Errorf(`failed to lock client: %w`, err))
		}
		defer locker.Unlock()
	}

	if !headless && !mc.CheckPlatform() {
		InfoBox("FriendNet Client", "It looks like this is your first time running the FriendNet client.\n\nThe web UI requires HTTPS and a custom certificate, so that will be installed now. If it is not installed, the web UI will not work in your browser.\n\nYou may be asked for your password a multiple times.\n\nYou may need to restart your browser afterward.")
		if err = mc.Install(); err != nil {
			logger.Error(`failed to install client root CA`, "err", err)
			InfoBox("Error", "Failed to install FriendNet client root CA. Please try again or install it manually by running the client with the -installca option.\n\nError: "+err.Error())
			os.Exit(1)
		}
	}

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
			httpsCertPem, httpsKeyPem, err = mc.GenCert([]string{webUrl.Hostname(), "localhost", "127.0.0.1"})
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

	updateChecker := updater.NewUpdateChecker(
		logger,
		appinfo.UpdateCheckerBaseUrl,
		appinfo.CurrentUpdate,
		appinfo.Ed25519Pubkey,
		appinfo.UpdateCheckerInterval,
	)
	go func() {
		for {
			newChan := updateChecker.NewUpdateChan()

			select {
			case <-ctx.Done():
				return
			case <-newChan:
				update, updateErr := updateChecker.GetNewUpdate()

				var newInfo *v1.UpdateInfo
				if updateErr != nil {
					newInfo = &v1.UpdateInfo{
						IsValid: false,
					}
				} else if update != nil {
					newInfo = &v1.UpdateInfo{
						IsValid:     true,
						CreatedTs:   update.CreatedTs,
						Version:     update.Version,
						Description: update.Description,
						Url:         update.Url,
					}
				} else {
					continue
				}

				eventBus.
					CreatePublisher(&v1.EventContext{}).
					Publish(&v1.Event{
						Type: v1.Event_TYPE_NEW_UPDATE,
						NewUpdate: &v1.Event_NewUpdate{
							Info: newInfo,
						},
					})
			}
		}
	}()

	downloadManager, err := client.NewDownloadManager(
		logger,
		multi,
		eventBus,
		store,
	)
	if err != nil {
		panic(fmt.Errorf(`failed to create download manager: %w`, err))
	}

	httpsKeyPair, err := tls.X509KeyPair(httpsCertPem, httpsKeyPem)
	if err != nil {
		panic(fmt.Errorf(`failed to parse HTTPS certificate key pair: %w`, err))
	}

	webServer := webserver.NewWebServer(
		logger,
		webserver.WithHttpsSupport(httpsKeyPair),
	)

	rpc, err := common.NewRpcServer(
		logger,
		webServer,
		common.RpcServerConfig{
			Address:             webAddr,
			AllowedMethods:      []string{"*"},
			BearerToken:         rpcBearerToken,
			CorsAllowAllOrigins: true,
		},
		client.NewRpcServer(
			logHandler,
			multi,
			eventBus,
			updateChecker,
			downloadManager,
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

	err = webServer.Mount(webAddr, "/content/", client.NewFileServer(logger, multi, rpcBearerToken))
	if err != nil {
		panic(fmt.Errorf(`failed to mount file proxy: %w`, err))
	}

	err = webServer.Mount(webAddr, "/", webui.Handler{})
	if err != nil {
		panic(fmt.Errorf(`failed to mount web UI: %w`, err))
	}

	metaCache := fsys.NewMetaCache(30*time.Second, 5*time.Minute)
	multiFs := multifs.NewMultiFs(multi,
		multifs.WithMetaCache(metaCache),
	)
	webdavHandler := &webdav.Handler{
		FileSystem: multifs.NewWebDavWrapper(multiFs),
		LockSystem: webdav.NewMemLS(),
	}
	err = webServer.Mount(davAddr, "/", webdavHandler)
	if err != nil {
		panic(fmt.Errorf(`failed to mount WebDAV handler: %w`, err))
	}

	// Close client on SIGTERM.
	var shutdownWg sync.WaitGroup
	defer stop()
	shutdownWg.Go(func() {
		<-ctx.Done()

		// Send stop event to all subscribers.
		eventBus.
			CreatePublisher(&v1.EventContext{}).
			Publish(&v1.Event{
				Type: v1.Event_TYPE_STOP,
			})
		time.Sleep(100 * time.Millisecond)

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
			_ = webServer.Close()
		})
		doWithTimeout(1*time.Second, func(_ context.Context) {
			_ = updateChecker.Close()
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

	if !noBrowser {
		// Try to open URL in browser.
		_ = browser.OpenURL(webUrlWithCreds)
	}

	logger.Info(`web UI server listening`,
		"addr", webAddr,
		"url", webUrlWithCreds,
		"token", rpcBearerToken,
	)

	go func() {
		serveErr := webServer.Serve()
		if serveErr != nil {
			if errors.Is(serveErr, http.ErrServerClosed) {
				return
			}
			logger.Error(`web server failed to serve`,
				"err", serveErr,
			)
		}

		stop()
	}()

	shutdownWg.Wait()

	if profilerFile != nil {
		pprof.StopCPUProfile()
		_ = profilerFile.Close()
		println("Profiler stopped")
	}
}
