package multifs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"friendnet.org/client"
	"friendnet.org/client/fsys"
	"friendnet.org/client/fsys/peerfs"
	"friendnet.org/client/room"
	"friendnet.org/common"
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

	var cursor int
	var slashIdx int

	slashIdx = strings.IndexRune(path, '/')
	if slashIdx == -1 {
		return res, true
	}
	{
		serverUuidPart := path[:slashIdx]
		spaceIdx := strings.LastIndexByte(serverUuidPart, ' ')
		if spaceIdx == -1 {
			return res, false
		}

		res.serverDirName = serverUuidPart
		res.serverUuid = serverUuidPart[:spaceIdx]
	}
	cursor += slashIdx + 1

	slashIdx = strings.IndexRune(path[cursor:], '/')
	if slashIdx == -1 {
		return res, true
	}
	{
		usernamePart := path[cursor : cursor+slashIdx]
		username, ok := common.NormalizeUsername(usernamePart)
		if !ok {
			return res, false
		}
		res.username = username
	}
	cursor += slashIdx + 1

	pathPart := path[cursor:]
	if pathPart == "" {
		res.path = common.RootProtoPath
		return res, true
	}

	// Sanitize and normalize path.

	pathPart = strings.TrimSuffix(pathPart, "/")

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

// RootFile is the fs.File returned when opening "/" or "".
type RootFile struct {
	mfs *MultiFs
}

func NewRootFile(mfs *MultiFs) RootFile {
	return RootFile{
		mfs: mfs,
	}
}

var _ fs.File = (*RootFile)(nil)
var _ fs.ReadDirFile = (*RootFile)(nil)

func (f RootFile) Stat() (fs.FileInfo, error) {
	return fsys.DummyDirFsWrapper("/"), nil
}
func (f RootFile) Read(_ []byte) (int, error) {
	return 0, errors.New("root directory is a directory, it cannot be read")
}
func (f RootFile) Close() error {
	return nil
}
func (f RootFile) ReadDir(n int) ([]fs.DirEntry, error) {
	servers := f.mfs.multi.GetAll()
	limit := min(n, len(servers))
	if limit < 1 {
		limit = len(servers)
	}

	entries := make([]fs.DirEntry, limit)
	for i := 0; i < limit; i++ {
		entries[i] = fsys.DummyDirFsWrapper(servers[i].Name)
	}

	return entries, nil
}

// ServerFile is the fs.File returned when opening a server directory.
type ServerFile struct {
	mfs     *MultiFs
	dirName string
	srv     *client.Server
}

func NewServerFile(mfs *MultiFs, dirName string, srv *client.Server) ServerFile {
	return ServerFile{
		mfs:     mfs,
		dirName: dirName,
		srv:     srv,
	}
}

var _ fs.File = (*ServerFile)(nil)
var _ fs.ReadDirFile = (*ServerFile)(nil)

func (f ServerFile) Stat() (fs.FileInfo, error) {
	return fsys.DummyDirFsWrapper(f.dirName), nil
}
func (f ServerFile) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("%q is a directory, it cannot be read", f.dirName)
}
func (f ServerFile) Close() error {
	return nil
}
func (f ServerFile) ReadDir(n int) ([]fs.DirEntry, error) {
	var limit int
	if n < 1 {
		n = 0
		limit = math.MaxInt
	} else {
		limit = n
	}

	ctx, cancel := f.mfs.mkCnTimeoutCtx()
	defer cancel()
	return client.DoValue[[]fs.DirEntry](f.srv.ConnNanny, ctx, func(_ context.Context, c *room.Conn) ([]fs.DirEntry, error) {
		stream, err := c.GetOnlineUsers()
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = stream.Close()
		}()

		var entries []fs.DirEntry
		if n > 0 {
			entries = make([]fs.DirEntry, 0, n)
		} else {
			entries = make([]fs.DirEntry, 0)
		}

		for {
			next, nextErr := stream.ReadNext()
			if nextErr != nil {
				if errors.Is(nextErr, io.EOF) {
					break
				}
				return nil, err
			}

			for _, user := range next.Users {
				if len(entries) >= limit {
					break
				}

				entries = append(entries, fsys.DummyDirFsWrapper(user.Username))
			}
		}

		return entries, nil
	})
}
