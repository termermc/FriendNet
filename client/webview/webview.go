package webview

import (
	"fmt"
	"log/slog"
	"net/url"
	"sync"

	"friendnet.org/common"
	"github.com/crgimenes/glaze"
)

// TODO The WebView struct should own a single instance of a webview and be able to open and close it independent of the
// client daemon.
// It should be able to have methods to navigate, open and close it.
// When a client sees a lock, it should send an RPC to open the webview.
// There should also be an RPC to navigate to trigger a navigation (which could be used for fnet:// links or something).

type WebView struct {
	logger *slog.Logger

	mu          sync.Mutex
	isDestroyed bool

	lastUrl  *url.URL
	rpcToken string
	rpcUrl   string

	isOpen bool
	handle glaze.WebView
}

// New creates a new WebView instance.
// It does not actually create a web view window until WebView.Open is called, so errors relating to opening it should
// be reported on the first Open call.
// The rpcUrl argument can be empty to use the same origin as the RPC.
func New(
	logger *slog.Logger,
	startUrl *url.URL,
	rpcToken string,
	rpcUrl string,
) *WebView {
	return &WebView{
		logger: logger.With("service", "webview.WebView"),

		lastUrl:  startUrl,
		rpcToken: rpcToken,
		rpcUrl:   rpcUrl,
	}
}

// Close destroys the webview.
// All calls after this will fail.
// Use Hide to close the webview without destroying this WebView instance.
func (w *WebView) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.isDestroyed {
		return nil
	}

	w.isOpen = false
	if w.isOpen && w.handle != nil {
		w.handle.Dispatch(func() {
			w.handle.Destroy()
		})
	}

	w.isDestroyed = true

	return nil
}

func (w *WebView) IsOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	return !w.isDestroyed && w.isOpen
}

func (w *WebView) urlWithToken(u *url.URL) *url.URL {
	res := *u

	q := res.Query()
	q.Set("token", w.rpcToken)
	if w.rpcUrl != "" {
		q.Set("rpc", w.rpcUrl)
	}

	res.RawQuery = q.Encode()
	return &res
}

// JS injected into the webview page.
// It does things like expose the fact that the web UI has access to native functions, and tracks the page's href.
const webviewJs = `
;(function() {
	window.__isNative = true;

	var lastLoc = '';
	window.__checkLocChanged = function () {
		var newLoc = window.location.href;
		if (newLoc === lastLoc) {
			return;
		}

		lastLoc = newLoc;
		__locChanged(newLoc);
	}

	__checkLocChanged();

	window.addEventListener('popstate', __checkLocChanged);
	window.addEventListener('hashchange', __checkLocChanged);
	setInterval(__checkLocChanged, 1000);
	var oldPushState = window.history.pushState
	window.history.pushState = (function(...args) {
		oldPushState.call(window.history, ...args);
		__checkLocChanged();
	}).bind(window.history);
})();
`

func (w *WebView) wireUp(h glaze.WebView) error {
	h.Init(webviewJs)
	return h.Bind("__locChanged", func(newUrl string) {
		u, err := url.Parse(newUrl)
		if err != nil {
			w.logger.Error("__locChanged called with invalid URL",
				"url", newUrl,
				"err", err,
			)
			return
		}

		w.mu.Lock()

		if u.Host != w.lastUrl.Host || u.Scheme != w.lastUrl.Scheme {
			w.logger.Error("__locChanged called with URL whose origin did not match the last URL; destroying webview",
				"newUrl", newUrl,
				"lastUrl", w.lastUrl.String(),
			)
			w.mu.Unlock()

			// The application should never navigate away from
			_ = w.Close()
			return
		}

		u.Query().Del("token")
		w.lastUrl = u

		w.mu.Unlock()
	})
}

// Open opens the webview, if not already open.
// If already open, it focuses the webview.
// Returns an error if opening the webview fails.
func (w *WebView) Open() error {
	//initErr := make(chan error)
	// TODO REMOVE THIS
	initErr := make(chan error, 1)

	w.mu.Lock()

	if w.isDestroyed || (w.isOpen && w.handle != nil) {
		w.handle.Dispatch(func() {
			w.handle.Focus()
		})
		w.mu.Unlock()
		return nil
	}

	// Initialize and run window in its own goroutine.
	// This is required, or else errors happen.
	//go func() {
	var err error
	w.handle, err = glaze.New(common.IsDebugMode)
	if err != nil {
		w.mu.Unlock()
		initErr <- err
		return err
	}

	w.handle.SetTitle("FriendNet Client")
	w.handle.SetSize(1280, 720, glaze.HintNone)
	w.handle.SetHtml("<h1>Loading...</h1>")
	if err = w.wireUp(w.handle); err != nil {
		initErr <- fmt.Errorf(`failed to wire up webview: %w`, err)
		return err
	}
	println(w.urlWithToken(w.lastUrl).String())
	w.handle.Navigate(w.urlWithToken(w.lastUrl).String())

	w.isOpen = true

	// Run window event loop.
	//go func() {
	hdl := w.handle
	w.mu.Unlock()

	hdl.Run()

	w.mu.Lock()
	w.isOpen = false
	w.handle = nil
	w.mu.Unlock()
	//}()
	//}()

	return <-initErr
}

// Hide hides the webview without destroying this WebView instance.
// If the webview is already hidden, this is a no-op.
func (w *WebView) Hide() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isOpen || w.handle == nil {
		return
	}

	w.isOpen = false
	w.handle.Dispatch(func() {
		w.handle.Eval("__checkLocChanged()")
		w.handle.Destroy()
	})
}

// Navigate navigates the webview to the specified URL.
// If the webview is not open, this is a no-op.
func (w *WebView) Navigate(u *url.URL) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.isDestroyed || !w.isOpen || w.handle == nil {
		return
	}

	w.handle.Dispatch(func() {
		w.handle.Navigate(w.urlWithToken(u).String())
	})
}
