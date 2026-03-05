package multifs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"friendnet.org/client"
	"friendnet.org/client/fsys"
	"friendnet.org/client/fsys/peerfs"
	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"golang.org/x/net/webdav"
)

// pathParts are the constituent parts of a path passed to MultiFs.
type pathParts struct {
	// The unparsed server directory name.
	// It could be something like "my server 6b74f955-e61f-4bcd-ae7a-49f246b85d46".
	// If serverDirName is set, serverUuid will also be set.
	serverDirName string
	serverUuid    string

	// If username is set, path will also be set.
	username common.NormalizedUsername
	path     common.ProtoPath
}

// Option is a function that configures a MultiFs.
type Option func(m *MultiFs)

// WithMetaCache configures a MultiFs to use a MetaCache for metadata caching.
//
// If cache is nil, metadata caching is disabled.
func WithMetaCache(cache *fsys.MetaCache) Option {
	return func(mfs *MultiFs) {
		mfs.cacheOrNil = cache
	}
}

// WithServerConnectTimeout configures a MultiFs to use the specified timeout for waiting for open server connections.
func WithServerConnectTimeout(timeout time.Duration) Option {
	return func(mfs *MultiFs) {
		mfs.nannyFsTimeout = timeout
	}
}

// MultiFs implements fs.FS in a way that provides a user-friendly, browsable filesystem for all
// servers and clients within them.
//
// It is stateless but can optionally use a MetaCache to cache metadata.
// It does not close any resources it uses, including the MetaCache.
//
// The path format is as follows:
// /<any string> <server UUID>/<username>/<path>...
//
// For example:
// /my server 6b74f955-e61f-4bcd-ae7a-49f246b85d46/someone/pics/funny_elephant.jpg
//
// In the above, the "my server" string is ignored and only the UUID is used to locate the server,
// the client's username is "someone", and the path is "/pics/funny_elephant.jpg"
//
// Browsing / should list all servers, and browsing a server should list all online clients in
// that server.
type MultiFs struct {
	mu sync.RWMutex

	multi *client.MultiClient

	cacheOrNil     *fsys.MetaCache
	nannyFsTimeout time.Duration
}

// NewMultiFs creates a new MultiFs instance with the specified MultiClient.
func NewMultiFs(multi *client.MultiClient, opts ...Option) *MultiFs {
	mfs := &MultiFs{
		multi: multi,
	}

	for _, opt := range opts {
		opt(mfs)
	}

	return mfs
}

var _ fs.FS = (*MultiFs)(nil)
var _ fs.StatFS = (*MultiFs)(nil)
var _ fs.ReadFileFS = (*MultiFs)(nil)
var _ fs.ReadDirFS = (*MultiFs)(nil)

// parseUrlPath parses a path in a URL and tries to get the relevant parts of it.
// If the path cannot be parsed, valid will be false.
//
// Always check for fields being zero!
// Zero or all fields may be zero, depending on how much of the fields the path covers.
//
// For example, "/my server 6b74f955-e61f-4bcd-ae7a-49f246b85d46/someone"
// will contain the server UUID field and username, but not the path.
//
// The path "/" will not contain any fields but is nevertheless valid.
func (mfs *MultiFs) parseUrlPath(path string) (res pathParts, valid bool) {
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	if strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}

	if path == "" || path == "." {
		return res, true
	}

	parts := strings.SplitN(path, "/", 3)

	if len(parts) >= 1 {
		serverPart := parts[0]
		spaceIdx := strings.LastIndexByte(serverPart, ' ')
		if spaceIdx == -1 || spaceIdx == len(serverPart)-1 {
			return res, false
		}
		res.serverDirName = serverPart
		res.serverUuid = serverPart[spaceIdx+1:]
	}

	if len(parts) >= 2 {
		usernamePart := parts[1]
		username, ok := common.NormalizeUsername(usernamePart)
		if !ok {
			return res, false
		}
		res.username = username
	}

	if len(parts) >= 3 {
		// Sanitize and normalize path.

		pathPart := strings.TrimSuffix(parts[2], "/")

		// Try to query unescape string.
		{
			unescaped, err := url.QueryUnescape(pathPart)
			if err == nil {
				pathPart = unescaped
			}
		}

		protoPath, pathErr := common.NormalizePath(pathPart)
		if pathErr != nil {
			return res, false
		}

		res.path = protoPath
	} else if len(parts) == 2 {
		res.path = common.RootProtoPath
	}

	return res, true
}

func (mfs *MultiFs) mkCnTimeoutCtx() (context.Context, context.CancelFunc) {
	if mfs.nannyFsTimeout == 0 {
		return context.Background(), func() {}
	}

	return context.WithTimeout(context.Background(), mfs.nannyFsTimeout)
}
func (mfs *MultiFs) mkNannyFs(srv *client.Server, username common.NormalizedUsername) (*peerfs.NannyFs, context.CancelFunc) {
	var opts []peerfs.Option
	if mfs.cacheOrNil != nil {
		opts = append(opts, peerfs.WithMetaCache(mfs.cacheOrNil, srv.Uuid+"/"+username.String()))
	}

	ctx, cancel := mfs.mkCnTimeoutCtx()
	return peerfs.NewNannyFs(ctx, srv.ConnNanny, username, opts...), cancel
}

func (mfs *MultiFs) Open(name string) (fs.File, error) {
	parts, partsOk := mfs.parseUrlPath(name)
	if !partsOk {
		return nil, fs.ErrNotExist
	}

	var srv *client.Server
	if parts.serverUuid != "" {
		var has bool
		srv, has = mfs.multi.GetByUuid(parts.serverUuid)
		if !has {
			return nil, fs.ErrNotExist
		}
	}

	if !parts.path.IsZero() {
		// Get file on peer.

		// It thinks srv might be nil, but I know that it isn't because path is not zero.
		//goland:noinspection GoMaybeNil
		nfs, cancel := mfs.mkNannyFs(srv, parts.username)
		defer cancel()

		return nfs.Open(parts.path.String())
	}

	if srv != nil {
		// Server folder opened.
		return NewServerFile(mfs, parts.serverDirName, srv), nil
	}

	// Root folder opened.
	return NewRootFile(mfs), nil
}

func (mfs *MultiFs) Stat(name string) (fs.FileInfo, error) {
	file, err := mfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	return file.Stat()
}

func (mfs *MultiFs) ReadFile(name string) ([]byte, error) {
	file, err := mfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	return io.ReadAll(file)
}

func (mfs *MultiFs) ReadDir(name string) ([]fs.DirEntry, error) {
	parts, partsOk := mfs.parseUrlPath(name)
	if !partsOk {
		return nil, fs.ErrNotExist
	}

	if parts.path.IsZero() {
		return nil, fmt.Errorf("%q is a directory, it cannot be read", name)
	}

	srv, has := mfs.multi.GetByUuid(parts.serverUuid)
	if !has {
		return nil, fs.ErrNotExist
	}

	nfs, cancel := mfs.mkNannyFs(srv, parts.username)
	defer cancel()

	return nfs.ReadDir(parts.path.String())
}

func NewRootFile(mfs *MultiFs) *fsys.DirWithChildrenFile {
	servers := mfs.multi.GetAll()
	entries := make([]fs.DirEntry, len(servers))
	for i, srv := range servers {
		entries[i] = fsys.DummyDirFsWrapper(srv.Name + " " + srv.Uuid)
	}

	return fsys.NewDirWithChildrenFile("/", entries)
}

// ServerFile is the fs.File returned when opening a server directory.
type ServerFile struct {
	mu sync.RWMutex

	mfs *MultiFs

	dirName string
	srv     *client.Server

	userStream protocol.Stream[*pb.MsgOnlineUsers]
	ended      bool
}

var _ fs.File = (*ServerFile)(nil)
var _ io.Seeker = (*ServerFile)(nil)
var _ fs.ReadDirFile = (*ServerFile)(nil)
var _ http.File = (*ServerFile)(nil)
var _ webdav.File = (*ServerFile)(nil)

func NewServerFile(mfs *MultiFs, dirName string, srv *client.Server) *ServerFile {
	return &ServerFile{
		mfs: mfs,

		dirName: dirName,
		srv:     srv,
	}
}

func (f *ServerFile) Stat() (fs.FileInfo, error) {
	return fsys.DummyDirFsWrapper(f.dirName), nil
}

func (f *ServerFile) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("%q is a directory, it cannot be read", f.dirName)
}

func (f *ServerFile) ReadDir(n int) ([]fs.DirEntry, error) {
	var limit int
	if n < 1 {
		n = 0
		limit = math.MaxInt
	} else {
		limit = n
	}

	f.mu.RLock()
	if f.ended {
		f.mu.RUnlock()

		if n > 0 {
			return nil, io.EOF
		}

		return nil, nil
	}

	stream := f.userStream
	f.mu.RUnlock()

	if stream == nil {
		ctx, cancel := f.mfs.mkCnTimeoutCtx()
		defer cancel()

		var err error
		stream, err = client.DoValue[protocol.Stream[*pb.MsgOnlineUsers]](f.srv.ConnNanny, ctx, func(_ context.Context, c *room.Conn) (protocol.Stream[*pb.MsgOnlineUsers], error) {
			return c.GetOnlineUsers()
		})
		if err != nil {
			return nil, err
		}

		f.mu.Lock()
		f.userStream = stream
		f.mu.Unlock()
	}

	var entries []fs.DirEntry
	if n > 0 {
		entries = make([]fs.DirEntry, 0, n)
	} else {
		entries = make([]fs.DirEntry, 0)
	}

	var wasEof bool
readLoop:
	for {
		next, nextErr := stream.ReadNext()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				wasEof = true
				_ = stream.Close()
				break
			}
			return entries, nextErr
		}

		for _, user := range next.Users {
			if len(entries) >= limit {
				break readLoop
			}

			entries = append(entries, fsys.DummyDirFsWrapper(user.Username))
		}
	}

	if wasEof {
		f.mu.Lock()
		f.ended = true
		f.mu.Unlock()

		if n > 0 {
			return entries, io.EOF
		}
	}

	return entries, nil
}

func (f *ServerFile) Readdir(count int) ([]fs.FileInfo, error) {
	entries, err := f.ReadDir(count)
	infos := make([]fs.FileInfo, len(entries))
	for i, entry := range entries {
		// DummyDirFsWrapper does not return an error on Info().
		info, _ := entry.Info()
		infos[i] = info
	}
	return infos, err
}

func (f *ServerFile) Seek(offset int64, whence int) (int64, error) {
	return fsys.DummySeek(offset, whence)
}

func (f *ServerFile) Write(_ []byte) (n int, err error) {
	return 0, fmt.Errorf(`file %q is a directory, it cannot be written to`, f.dirName)
}

func (f *ServerFile) Close() error {
	f.mu.RLock()
	stream := f.userStream
	f.mu.RUnlock()

	if stream != nil {
		return stream.Close()
	}

	return nil
}
