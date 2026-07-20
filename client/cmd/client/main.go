package main

import (
	"bytes"
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/client"
	"friendnet.org/client/cert"
	"friendnet.org/client/clog"
	"friendnet.org/client/direct"
	"friendnet.org/client/event"
	"friendnet.org/client/fsys"
	"friendnet.org/client/fsys/multifs"
	"friendnet.org/client/storage"
	"friendnet.org/client/webview"
	"friendnet.org/common"
	"friendnet.org/common/machine"
	"friendnet.org/common/webserver"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
	"friendnet.org/updater"
	"friendnet.org/webui"
	"github.com/pkg/browser"
	"golang.org/x/net/webdav"
)

const lockFilename = "client-lock.json"

type LockData struct {
	Ts      int64  `json:"ts"`
	RpcAddr string `json:"rpc_addr"`

	// May be empty.
	WebViewAddr string `json:"webview_addr"`
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

	// See if we can dial the addresses in the lock.
	if !common.TryTcpHost(rpcUrl.Host, 1*time.Second) {
		// Failed to dial address; this is probably a stale lock.
		_ = os.Remove(filePath)
		return nil
	}

	return &data
}

// Lock creates a lock file.
// webViewAddr can be empty for none.
func (l *Locker) Lock(rpcAddr string, webViewAddr string) error {
	filePath := filepath.Join(l.lockDir, lockFilename)
	data := LockData{
		Ts:          time.Now().UnixMilli(),
		RpcAddr:     rpcAddr,
		WebViewAddr: webViewAddr,
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
	var noWebView bool
	var openBrowser bool
	var noLock bool
	var resetToken bool
	var pprofFile string
	var rmCertHost string

	flag.StringVar(&dataDir, "datadir", "", "path to the client's data directory")
	flag.StringVar(&webAddr, "webaddr", "https://127.0.0.1:20042", "web UI and RPC address")
	flag.StringVar(&davAddr, "davaddr", "https://127.0.0.1:20043", "WebDAV server address")
	flag.BoolVar(&noWebView, "nowebview", false, "do not open a webview window")
	flag.BoolVar(&openBrowser, "openbrowser", false, "opens the web UI in the browser at startup")
	flag.BoolVar(&noLock, "nolock", false, "do not use a lock to prevent multiple instances of the client from running")
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
		openBrowser = false
		noWebView = true
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

	var webView *webview.WebView
	var webViewAddr string
	if !noWebView {
		// Let the OS choose an open port for us to use.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(fmt.Errorf(`failed to listen on random port: %w`, err))
		}
		listenPort := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()

		webViewAddr = "http://127.0.0.1:" + strconv.Itoa(listenPort)
		addrUrl, _ := url.Parse(webViewAddr)

		webView = webview.New(logger, addrUrl, rpcBearerToken)
	}

	if !noLock {
		locker := &Locker{
			lockDir: dataDir,
		}
		lockData := locker.CheckLock()
		if lockData != nil {
			println("Client is already running")

			if openBrowser {
				// Try to open web UI in browser.
				_ = browser.OpenURL(webUrlWithCreds)
			}

			_ = logHandler.Close()
			_ = store.Close()

			return
		}

		err = locker.Lock(webAddr, webViewAddr)
		if err != nil {
			panic(fmt.Errorf(`failed to lock client: %w`, err))
		}
		defer locker.Unlock()
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
			httpsCert, err := common.GenSelfSignedPem(webUrl.Hostname(), true)
			if err != nil {
				panic(fmt.Errorf(`failed to generate HTTPS certificate: %w`, err))
			}

			privkeyPrefix := []byte("-----BEGIN PRIVATE KEY-----")
			privKeyIdx := bytes.Index(httpsCert, privkeyPrefix)
			if privKeyIdx == -1 {
				panic(fmt.Errorf(`BUG: failed to find private key in HTTPS certificate`))
			}

			httpsCertPem = httpsCert[:privKeyIdx]
			httpsKeyPem = httpsCert[privKeyIdx:]

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
		updater.UpdateCheckerBaseUrl,
		updater.CurrentUpdate,
		updater.Ed25519Pubkey,
		updater.UpdateCheckerInterval,
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

	// Set up web UI and RPC handler.
	// It will listen on the configured or default address on HTTPS, and also on a random port to be used by the
	// webview.
	webAddrs := make([]string, 0, 2)
	webAddrs = append(webAddrs, webAddr)
	if webViewAddr != "" {
		webAddrs = append(webAddrs, webViewAddr)
	}

	rpc, err := common.NewRpcServer(
		logger,
		webServer,
		common.RpcServerConfig{
			Addresses:           webAddrs,
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
			store,
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

	for _, addr := range webAddrs {
		err = webServer.Mount(addr, "/content/", client.NewFileServer(logger, multi, rpcBearerToken))
		if err != nil {
			panic(fmt.Errorf(`failed to mount file proxy: %w`, err))
		}
		err = webServer.Mount(addr, "/", webui.Handler{})
		if err != nil {
			panic(fmt.Errorf(`failed to mount web UI: %w`, err))
		}
	}

	// Set up WebDAV handler.
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

		if webView != nil {
			_ = webView.Close()
		}

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

	if openBrowser {
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

	// TODO REMOVE THIS
	//glz, err := glaze.New(true)
	//if err != nil {
	//	logger.Error(`failed to create glaze client`,
	//		"err", err,
	//	)
	//	return
	//}

	if webView != nil {
		//go func() {
		// Wait until the HTTP address is available.
		addr, _ := url.Parse(webViewAddr)
		for {
			ok := common.TryTcpHost(addr.Host, 10*time.Second)
			if ok {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		wvErr := webView.Open()
		if wvErr != nil {
			var wvEngine string
			switch runtime.GOOS {
			case "darwin":
				wvEngine = "WKWebView"
			case "linux":
				wvEngine = "WebKitGTK (is it installed?)"
			case "windows":
				wvEngine = "WebView2 (is Windows up-to-date?)"
			}

			errMsg := "Failed to open web view: " + wvErr.Error() + "\n\n" +
				"Your system may not have the components required to show a web view.\n" +
				"Required component: " + wvEngine + "\n\n" +
				"You can still navigate to the web UI in your browser: " + webUrlWithCreds + "\n" +
				"A certificate error is normal due to using a self-signed certificate and can safely be accepted."

			logger.Error(`failed to open webview`,
				"err", wvErr,
			)

			InfoBox("FriendNet Web View Failed", errMsg)
		}
		//}()
	}

	shutdownWg.Wait()

	if profilerFile != nil {
		pprof.StopCPUProfile()
		_ = profilerFile.Close()
		println("Profiler stopped")
	}
}
