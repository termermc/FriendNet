package client

import (
	"archive/zip"
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
	"time"

	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
	"golang.org/x/net/http2"
)

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

	const schemeMsg = "Files are served based on the path scheme: /content/:SERVER/:USERNAME/:PATH..."
	const indexMsg = "Hi, you've reached the peer proxy HTTP server.\n\n" + schemeMsg + "\n\nPossible query parameter options:\n - ?download=1 signals for the browser to download the file\n - ?allowCache=1 sets caching headers to allow browser to cache the file\n - ?zip=1 on a directory downloads a zip of the directory's contents\n\nHave fun!\n"

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
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")

	if url.Query().Has("allowCache") {
		w.Header().Set("Cache-Control", "private, max-age=600, must-revalidate")
		w.Header().Set("Expires", time.Now().Add(10*time.Minute).Format(http.TimeFormat))
	} else {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}

	// Advertise range support.
	w.Header().Set("Accept-Ranges", "bytes")

	if url.Path == "/" {
		text(w, r, http.StatusOK, indexMsg)
		return
	}

	if !strings.HasPrefix(url.Path, "/content/") {
		http.NotFound(w, r)
		return
	}

	pathParts := strings.Split(strings.TrimSuffix(url.Path[1:], "/"), "/")

	if len(pathParts) < 3 {
		text(w, r, http.StatusBadRequest, schemeMsg+"\n")
		return
	}

	serverUuid := pathParts[1]
	usernameRaw := pathParts[2]
	pathRaw := "/" + strings.Join(pathParts[3:], "/")
	path, err := common.ValidatePath(pathRaw)
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
		peer := c.GetVirtualC2cConn(username, false)

		// Get metadata before getting file.
		// This is necessary for range requests.
		var meta *pb.MsgFileMeta
		meta, err = peer.GetFileMeta(path)
		if err != nil {
			if errors.Is(err, protocol.ErrPeerUnreachable) {
				text(w, r, http.StatusBadGateway, "peer unreachable\n")
				return nil
			}

			if msgErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
				if msgErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
					text(w, r, http.StatusNotFound, "file not found\n")
					return nil
				}
			}

			return err
		}

		if meta.IsDir {
			doZip := url.Query().Has("zip")
			if !doZip {
				text(w, r, http.StatusNotImplemented, "Path points to a directory.\n\nTo download the directory's content as a zip, specify ?zip=1.\n")
				return nil
			}

			// Zip folder contents.

			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, meta.Name))

			// We will scan through the directory while writing the files to a zip output stream.
			// The scanning will happen in the background while we receive the entries in this thread and write them.

			zipCtx, cancel := context.WithCancelCause(ctx)

			type zipEntry struct {
				path common.ProtoPath
				meta *pb.MsgFileMeta
			}

			entries := make(chan zipEntry, 1_000)

			go func() {
				toCrawl := []common.ProtoPath{path}

				for len(toCrawl) > 0 {
					crawlPath := toCrawl[0]
					toCrawl = toCrawl[1:]

					fileStream, streamErr := peer.GetDirFiles(crawlPath)
					if streamErr != nil {
						if errors.Is(streamErr, protocol.ErrPeerUnreachable) {
							cancel(protocol.ErrPeerUnreachable)
							return
						}

						cancel(fmt.Errorf(`failed to read contents in directory %q: %w`, crawlPath.String(), streamErr))
					}

					for {
						next, nextErr := fileStream.ReadNext()
						if nextErr != nil {
							if errors.Is(nextErr, io.EOF) {
								break
							}

							if protoErr, ok := errors.AsType[protocol.ProtoMsgError](nextErr); ok {
								// File might change while we are crawling it.
								// Just skip errors that might happen if the directory is changing.
								if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST ||
									protoErr.Msg.Type == pb.ErrType_ERR_TYPE_PATH_NOT_DIRECTORY {
									break
								}
							}

							_ = fileStream.Close()
							cancel(fmt.Errorf(`failed to read next file in directory %q: %w`, crawlPath.String(), nextErr))
							return
						}

						for _, file := range next.Files {
							filePath := common.UncheckedCreateProtoPath(crawlPath.String() + "/" + file.Name)

							entries <- zipEntry{
								path: filePath,
								meta: file,
							}

							if file.IsDir {
								toCrawl = append(toCrawl, filePath)
							}
						}
					}
					_ = fileStream.Close()
				}

				close(entries)
			}()

			zipErr := func() error {
				zw := zip.NewWriter(w)

			entryLoop:
				for {
					select {
					case <-zipCtx.Done():
						if ctxErr := zipCtx.Err(); ctxErr != nil {
							return ctxErr
						}

						// If the context was canceled without an error, something weird happened.
						return errors.New("directory zip streaming context canceled without an error, this should not happen")

					case entry := <-entries:
						if entry.meta == nil {
							// No more entries.
							break entryLoop
						}

						entryPath := strings.TrimPrefix(entry.path.String(), path.String())[1:]

						if entry.meta.IsDir {
							_, fileErr := zw.Create(entryPath + "/")
							if fileErr != nil {
								return fileErr
							}
							continue
						}

						fileW, fileErr := zw.Create(entryPath)
						if fileErr != nil {
							return fileErr
						}

						_, reader, getErr := peer.GetFile(&pb.MsgGetFile{
							Path: entry.path.String(),
						})
						if getErr != nil {
							return getErr
						}

						_, copyErr := io.Copy(fileW, reader)
						if copyErr != nil {
							return copyErr
						}
					}
				}

				return zw.Close()
			}()
			if zipErr != nil {
				s.logger.Error("error while streaming directory zip",
					"service", "client.FileServerHandler",
					"server", serverUuid,
					"username", username.String(),
					"path", path.String(),
					"err", zipErr,
				)

				hijacker, ok := w.(http.Hijacker)
				if ok {
					// Force-close the connection so the client knows that the sending failed.
					conn, _, _ := hijacker.Hijack()
					if conn != nil {
						_ = conn.Close()
					}
				}

				return nil
			}
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
		if opErr, ok := errors.AsType[*net.OpError](err); ok {
			if errors.Is(opErr.Err, syscall.ECONNRESET) {
				// Nothing to report, the HTTP client closed the connection.
				return
			}
		}
		if _, ok := errors.AsType[http2.GoAwayError](err); ok {
			return
		}
		if _, ok := errors.AsType[http2.StreamError](err); ok {
			return
		}
		if _, ok := errors.AsType[http2.ConnectionError](err); ok {
			return
		}
		if _, ok := errors.AsType[*quic.StreamError](err); ok {
			return
		}
		if strings.Contains(err.Error(), "http2: stream closed") {
			return
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
