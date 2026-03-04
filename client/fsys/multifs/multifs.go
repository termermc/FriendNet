package multifs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"time"

	"friendnet.org/client"
	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

type pathParts struct {
	// If serverDirName is set, serverUuid will also be set.
	serverDirName string
	serverUuid    string

	// If username is set, path will also be set.
	username common.NormalizedUsername
	path     common.ProtoPath
}

// MultiFs implements fs.FS in a way that provides a user-friendly, browsable filesystem for all
// servers and clients within them.
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
}

// NewMultiFs creates a new MultiFs instance with the specified MultiClient.
func NewMultiFs(multi *client.MultiClient) *MultiFs {
	ctx, ctxCancel := context.WithCancel(context.Background())

	m := &MultiFs{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		cache:                   make(map[string]metaCacheEntry),
		cacheEntryValidDuration: 10 * time.Second,
		cacheGcInterval:         5 * time.Minute,

		multi: multi,
	}

	go m.cacheGc()

	return m
}

func (m *MultiFs) mkKey(parts pathParts) string {
	if parts.serverUuid == "" {
		panic("BUG: mkKey: serverUuid is empty")
	}
	if parts.username.IsZero() {
		panic("BUG: mkKey: username is zero")
	}
	if parts.path.IsZero() {
		panic("BUG: mkKey: path is zero")
	}

	return parts.serverUuid + "/" + parts.username.String() + "/" + parts.path.String()
}

func (m *MultiFs) putCache(parts pathParts, meta *pb.MsgFileMeta) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.mkKey(parts)

	m.cache[key] = metaCacheEntry{
		meta:  meta,
		expTs: time.Now().Add(m.cacheEntryValidDuration),
	}
}

// getMeta returns metadata for the specified path.
// All fields must be filled.
//
// If a cache entry exists, it will be returned.
// If not, the metadata will be fetched from the relevant peer and the cache will be filled.
//
// If the file does not exist, returns fs.ErrNotExist.
func (m *MultiFs) getMeta(ctx context.Context, parts pathParts) (*pb.MsgFileMeta, error) {
	cacheKey := m.mkKey(parts)

	// Check for cache entry.
	m.mu.RLock()
	entry, has := m.cache[cacheKey]
	m.mu.RUnlock()

	if has && entry.expTs.After(time.Now()) {
		return entry.meta, nil
	}

	// No cached entry.

	srv, has := m.multi.GetByUuid(parts.serverUuid)
	if !has {
		return nil, fs.ErrNotExist
	}

	return DoValue[*pb.MsgFileMeta](srv.ConnNanny, ctx, func(ctx context.Context, c *room.Conn) (*pb.MsgFileMeta, error) {
		peer := c.GetVirtualC2cConn(parts.username, false)

		meta, err := peer.GetFileMeta(parts.path)
		if err != nil {
			if errors.Is(err, protocol.ErrPeerUnreachable) {
				return nil, fs.ErrNotExist
			}
			if protoErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
				if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
					return nil, fs.ErrNotExist
				}
			}

			return nil, err
		}

		// Put cache entry.
		m.putCache(parts, meta)

		return meta, nil
	})
}

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
func (m *MultiFs) parseUrlPath(path string) (res pathParts, valid bool) {
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

func (m *MultiFs) Open(name string) (fs.File, error) {
	// TODO Get meta if applicable.
}

var _ fs.FS = (*MultiFs)(nil)
var _ fs.StatFS = (*MultiFs)(nil)
var _ fs.ReadFileFS = (*MultiFs)(nil)
var _ fs.ReadDirFS = (*MultiFs)(nil)

type MultiFsFile struct {
	mfs *MultiFs

	parts pathParts

	// meta should only be set if parts.path is not zero.
	// The constructor must check if the file exists before making this struct.
	meta *pb.MsgFileMeta

	curReader io.ReadCloser
}

func (m MultiFsFile) Stat() (fs.FileInfo, error) {
	if !m.parts.path.IsZero() {
		// File exists, send its meta.
		return MetaToFs(m.meta), nil
	}

	if m.parts.serverUuid != "" {
		// Check if server exists and return dummy folder.
		_, has := m.mfs.multi.GetByUuid(m.parts.serverUuid)
		if !has {
			return nil, fs.ErrNotExist
		}

		return DummyDirFsWrapper(m.parts.serverDirName), nil
	}

	// Root, return dummy folder.
	return DummyDirFsWrapper("/"), nil
}

func (m MultiFsFile) Read(bytes []byte) (int, error) {
	if m.meta == nil {
		return 0, io.EOF
	}
	if m.meta.IsDir {
		return 0, io.EOF
	}

	if m.curReader != nil {
		return m.curReader.Read(bytes)
	}

	// TODO Try to get file and set reader.
}

func (m MultiFsFile) Close() error {
	if m.curReader != nil {
		return m.curReader.Close()
	}
	return nil
}

var _ fs.File = (*MultiFsFile)(nil)
var _ fs.ReadDirFile = (*MultiFsFile)(nil)
var _ io.ReaderAt = (*MultiFsFile)(nil)
