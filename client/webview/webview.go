package webview

import (
	"friendnet.org/common"
	"github.com/crgimenes/glaze"
)

// TODO The WebView struct should own a single instance of a webview and be able to open and close it independent of the
// client daemon.
// It should be able to have methods to navigate, open and close it.
// When a client sees a lock, it should send an RPC to open the webview.
// There should also be an RPC to navigate to trigger a navigation (which could be used for fnet:// links or something.

type WebView struct {
	startUrl string
	handle   glaze.WebView
}

func NewWebView(startUrl string) (*WebView, error) {
	handle, err := glaze.New(common.IsDebugMode)
	if err != nil {
		return nil, err
	}

	return &WebView{
		startUrl: startUrl,
		handle:   handle,
	}, nil
}

func (w *WebView) Close() error {
	w.handle.Destroy()
	return nil
}

// Run runs the webview.
// The WebView must be closed after this function exits.
func (w *WebView) Run() {
	w.handle.SetTitle("FriendNet Client")
	w.handle.SetSize(800, 600, glaze.HintNone)
	w.handle.Navigate(w.startUrl)
	w.handle.Run()
}
