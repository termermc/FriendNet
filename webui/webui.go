//go:generate npm ci
//go:generate npm run build

package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var distDir embed.FS

// Dist contains the embedded web UI files.
var Dist = func() fs.FS {
	dist, _ := fs.Sub(distDir, "dist")
	return dist
}()

// Handler is an http.Handler that serves the web UI with the proper security headers.
type Handler struct {
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: http: https:; font-src 'self' data:; connect-src 'self' http: https:; media-src 'self' data: http: https:; frame-src 'self' http: https:")

	http.FileServer(http.FS(Dist)).ServeHTTP(w, r)
}

var _ http.Handler = (*Handler)(nil)
