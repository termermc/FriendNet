package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// TODO Protect with some kind of token?
// Maybe the URL should include the token as the first segment or something.
// I want to make sure that relative paths can load so that a profile page can be iframe'd.

// FileServerHandler is an HTTP handler that serves files from remote peers.
type FileServerHandler struct {
	logger *slog.Logger
	multi  *MultiClient
}

func NewFileServer(
	logger *slog.Logger,
	multi *MultiClient,
) *FileServerHandler {
	return &FileServerHandler{
		logger: logger,
		multi:  multi,
	}
}

var _ http.Handler = (*FileServerHandler)(nil)

func (s *FileServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wroteHeader := false
	text := func(w http.ResponseWriter, r *http.Request, status int, text string) {
		if wroteHeader {
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		wroteHeader = true

		switch r.Method {
		case http.MethodHead, http.MethodOptions:
			return
		}

		_, _ = w.Write([]byte(text))
	}
	internalError := func(w http.ResponseWriter, r *http.Request, err error) {
		text(w, r, http.StatusInternalServerError, fmt.Sprintf("internal error:\n\n%v\n", err))
	}

	const schemeMsg = "Files are served based on the path scheme: /SERVER/USERNAME/PATH"
	const indexMsg = "Hi, you've reached the peer proxy HTTP server.\n" + schemeMsg + "\nYou can specify ?download=1 to signal browsers to download the file.\nHave fun!\n"

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		break
	default:
		text(w, r, http.StatusMethodNotAllowed, "method not allowed\n")
		return
	}

	isHead := r.Method == http.MethodHead
	url := r.URL

	// Strict CSP for pages served from peers.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; base-uri 'none'; form-action 'none'; sandbox")

	// Prevent browser caching.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	if url.Path == "/" {
		text(w, r, http.StatusOK, indexMsg)
		return
	}

	parts := strings.Split(strings.TrimSuffix(url.Path[1:], "/"), "/")

	if len(parts) < 3 {
		text(w, r, http.StatusBadRequest, schemeMsg+"\n")
		return
	}

	serverUuid := parts[0]
	usernameRaw := parts[1]
	pathRaw := "/" + strings.Join(parts[2:], "/")
	path, err := protocol.ValidatePath(pathRaw)
	if err != nil {
		text(w, r, http.StatusBadRequest, fmt.Sprintf("invalid path %q: %v\n", pathRaw, err))
		return
	}

	username, usernameOk := common.NormalizeUsername(usernameRaw)
	if !usernameOk {
		text(w, r, http.StatusBadRequest, fmt.Sprintf("invalid username %q\n", usernameRaw))
		return
	}

	server, has := s.multi.GetByUuid(serverUuid)
	if !has {
		text(w, r, http.StatusNotFound, fmt.Sprintf("no such server %q\n", serverUuid))
		return
	}

	err = server.Do(r.Context(), func(ctx context.Context, c *room.Conn) error {
		peer := c.GetVirtualC2cConn(username)

		var meta *pb.MsgFileMeta
		var reader io.ReadCloser
		meta, reader, err = peer.GetFile(&pb.MsgGetFile{
			Path: path.String(),

			// TODO Ranges
			Offset: 0,
			Limit:  0,
		})
		if err != nil {
			if errors.Is(err, protocol.ErrPeerUnreachable) {
				text(w, r, http.StatusBadGateway, "peer unreachable\n")
				return nil
			}

			return err
		}
		defer func() {
			_ = reader.Close()
		}()
		go func() {
			select {
			case <-ctx.Done():
				_ = reader.Close()
			}
		}()

		if meta.IsDir {
			text(w, r, http.StatusNotImplemented, "path points to a directory\n")
			return nil
		}

		// TODO Ranges

		// Great, we've got the file.
		// Now we can set the necessary headers to serve it.
		fileExt := filepath.Ext(path.String())
		mimeType := mime.TypeByExtension(fileExt)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.Size))

		if url.Query().Has("download") {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.Name))
		}

		if isHead {
			return nil
		}

		// Write it!
		wroteHeader = true
		_, err = io.Copy(w, reader)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		s.logger.Error("failed to get file from peer",
			"service", "client.FileServerHandler",
			"server", serverUuid,
			"username", usernameRaw,
			"path", pathRaw,
			"err", err,
		)
		internalError(w, r, err)
		return
	}
}
