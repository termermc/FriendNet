package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

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

	// Allow fetching files from it.
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Strict CSP for pages served from peers.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; media-src 'self' data:; base-uri 'none'; form-action 'none'; sandbox")

	// Prevent browser caching.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Advertise range support.
	w.Header().Set("Accept-Ranges", "bytes")

	if url.Path == "/" {
		text(w, r, http.StatusOK, indexMsg)
		return
	}

	pathParts := strings.Split(strings.TrimSuffix(url.Path[1:], "/"), "/")

	if len(pathParts) < 3 {
		text(w, r, http.StatusBadRequest, schemeMsg+"\n")
		return
	}

	serverUuid := pathParts[0]
	usernameRaw := pathParts[1]
	pathRaw := "/" + strings.Join(pathParts[2:], "/")
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

		// Get metadata before getting file.
		// This is necessary for range requests.
		var meta *pb.MsgFileMeta
		meta, err = peer.GetFileMeta(path)
		if err != nil {
			if errors.Is(err, protocol.ErrPeerUnreachable) {
				text(w, r, http.StatusBadGateway, "peer unreachable\n")
				return nil
			}

			var msgErr protocol.ProtoMsgError
			if errors.As(err, &msgErr) {
				if msgErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
					text(w, r, http.StatusNotFound, "file not found\n")
					return nil
				}
			}

			return err
		}

		if meta.IsDir {
			text(w, r, http.StatusNotImplemented, "path points to a directory\n")
			return nil
		}

		fileExt := filepath.Ext(path.String())
		mimeType := mime.TypeByExtension(fileExt)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", mimeType)

		if url.Query().Has("download") {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.Name))
		}

		// Parse range.
		rangeHeader := r.Header.Get("Range")
		fileSize := int64(meta.Size)
		offset, limit, rangeOk := common.ParseHttpRange(rangeHeader, fileSize)
		if !rangeOk {
			text(w, r, http.StatusBadRequest, "invalid range string\n")
		}

		// Check if range can be satisfied.
		if offset+limit > fileSize {
			text(w, r, http.StatusRequestedRangeNotSatisfiable, fmt.Sprintf("requested range not satisfiable\n"))
		}

		{
			var end int64
			if limit == 0 {
				end = fileSize - 1
			} else {
				end = offset + limit - 1
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, fileSize))
		}

		{
			var contentLen int64
			if limit == 0 {
				contentLen = fileSize
			} else {
				contentLen = limit
			}

			w.Header().Set("Content-Length", strconv.FormatInt(contentLen, 10))
		}

		var reader io.ReadCloser
		_, reader, err = peer.GetFile(&pb.MsgGetFile{
			Path: path.String(),

			Offset: uint64(offset),
			Limit:  uint64(limit),
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

		if rangeHeader != "" {
			w.WriteHeader(http.StatusPartialContent)
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
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			if errors.Is(opErr.Err, syscall.ECONNRESET) {
				// Nothing to report, the HTTP client closed the connection.
				return
			}
		}

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
