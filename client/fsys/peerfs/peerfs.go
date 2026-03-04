package peerfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sync"
	"time"

	"friendnet.org/client/fsys"
	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// Option is a function that configures a PeerFs.
type Option func(pfs *PeerFs)

// WithMetaCache configures a PeerFs to use a MetaCache for metadata caching.
// The cache is keyed by the provided keyPrefix.
//
// If cache is nil, metadata caching is disabled.
func WithMetaCache(cache *fsys.MetaCache, keyPrefix string) Option {
	return func(pfs *PeerFs) {
		pfs.cacheOrNil = cache
		pfs.cachePrefix = keyPrefix
	}
}

// PeerFs implements fs.FS that exposes a peer's shares.
// It is stateless but can optionally use a MetaCache to cache metadata.
//
// Closing PeerFs is a no-op and does not close the cache it uses, if any.
type PeerFs struct {
	roomConn *room.Conn
	peer     room.VirtualC2cConn
	username common.NormalizedUsername

	cacheOrNil  *fsys.MetaCache
	cachePrefix string
}

// NewPeerFs creates a new PeerFs with the specified room connection, peer username, and options.
func NewPeerFs(roomConn *room.Conn, username common.NormalizedUsername, opts ...Option) *PeerFs {
	pfs := &PeerFs{
		roomConn: roomConn,
		peer:     roomConn.GetVirtualC2cConn(username, false),
		username: username,
	}

	for _, opt := range opts {
		opt(pfs)
	}

	return pfs
}

var _ fs.FS = (*PeerFs)(nil)
var _ fs.StatFS = (*PeerFs)(nil)
var _ fs.ReadFileFS = (*PeerFs)(nil)
var _ fs.ReadDirFS = (*PeerFs)(nil)

func (pfs *PeerFs) refineError(err error) error {
	if errors.Is(err, protocol.ErrPeerUnreachable) {
		return fs.ErrNotExist
	}
	if protoErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
		if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
			return fs.ErrNotExist
		}
	}

	return err
}

// getMeta returns the metadata and resolved path for the specified path string.
// It returns fs.ErrNotExist if the file does not exist.
// Tries to use the cache if available.
// Its error return value does not need to be refined.
func (pfs *PeerFs) getMeta(pathStr string) (*pb.MsgFileMeta, common.ProtoPath, error) {
	path, err := common.NormalizePath(pathStr)
	if err != nil {
		return nil, common.ZeroProtoPath, fs.ErrInvalid
	}

	var meta *pb.MsgFileMeta

	// First, check for cached entry.
	if pfs.cacheOrNil != nil {
		meta, _ = pfs.cacheOrNil.Get(pfs.cachePrefix, path)
		if meta != nil {
			return meta, path, nil
		}
	}

	// Get from peer.
	meta, err = pfs.peer.GetFileMeta(path)
	if err != nil {
		return nil, common.ZeroProtoPath, pfs.refineError(err)
	}

	if pfs.cacheOrNil != nil {
		pfs.cacheOrNil.Set(pfs.cachePrefix, path, meta)
	}

	return meta, path, nil
}

// Open returns a DirFile or RegularFile for the specified path.
// Its error return value does not need to be refined.
func (pfs *PeerFs) Open(name string) (fs.File, error) {
	meta, path, err := pfs.getMeta(name)
	if err != nil {
		return nil, err
	}

	if meta.IsDir {
		return NewPeerFsDirFile(pfs, path, meta), nil
	}

	return NewRegularFile(pfs, path, meta), nil
}

func (pfs *PeerFs) Stat(name string) (fs.FileInfo, error) {
	meta, _, err := pfs.getMeta(name)
	if err != nil {
		return nil, err
	}

	return MetaToFs(meta), nil
}

func (pfs *PeerFs) ReadFile(name string) ([]byte, error) {
	file, err := pfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	return io.ReadAll(file)
}

func (pfs *PeerFs) ReadDir(name string) ([]fs.DirEntry, error) {
	meta, path, err := pfs.getMeta(name)
	if err != nil {
		return nil, err
	}

	if !meta.IsDir {
		return nil, fmt.Errorf(`tried to get files in peer %q path %q, but it was not directory`, pfs.username.String(), path.String())
	}

	file := NewPeerFsDirFile(pfs, path, meta)
	defer func() {
		_ = file.Close()
	}()
	return file.ReadDir(0)
}

// MetaFsWrapper wraps a *pb.MsgFileMeta and implements fs.FileInfo and fs.DirEntry.
// It should be passed by value, not by reference.
type MetaFsWrapper struct {
	meta *pb.MsgFileMeta
}

var _ fs.FileInfo = MetaFsWrapper{}
var _ fs.DirEntry = MetaFsWrapper{}

// MetaToFs wraps the provided *pb.MsgFileMeta in a type that implements fs.FileInfo and fs.DirEntry.
func MetaToFs(meta *pb.MsgFileMeta) MetaFsWrapper {
	return MetaFsWrapper{meta: meta}
}

func (p MetaFsWrapper) Name() string {
	return p.meta.Name
}
func (p MetaFsWrapper) Size() int64 {
	return int64(p.meta.Size)
}
func (p MetaFsWrapper) Mode() fs.FileMode {
	if p.meta.IsDir {
		return fs.ModeDir | fsys.FsFilePerms
	}
	return fsys.FsFilePerms
}
func (p MetaFsWrapper) ModTime() time.Time {
	return fsys.FsModTime
}
func (p MetaFsWrapper) IsDir() bool {
	return p.meta.IsDir
}
func (p MetaFsWrapper) Sys() any {
	return nil
}
func (p MetaFsWrapper) Type() fs.FileMode {
	if p.meta.IsDir {
		return fs.ModeDir
	}
	return 0
}
func (p MetaFsWrapper) Info() (fs.FileInfo, error) {
	return p, nil
}

// RegularFile represents a regular, non-directory file shared by a peer.
// It implements fs.File and io.Seeker, and it makes GetFile calls to the peer under the hood.
// Seeking closes the current reader from the last GetFile call, if any.
type RegularFile struct {
	mu sync.RWMutex

	pfs *PeerFs

	path common.ProtoPath
	meta *pb.MsgFileMeta

	readCursor int64
	curReader  io.ReadCloser
}

func NewRegularFile(pfs *PeerFs, path common.ProtoPath, meta *pb.MsgFileMeta) *RegularFile {
	return &RegularFile{
		pfs: pfs,

		path: path,
		meta: meta,
	}
}

var _ fs.File = (*RegularFile)(nil)
var _ io.Seeker = (*RegularFile)(nil)

func (f *RegularFile) Stat() (fs.FileInfo, error) {
	return MetaToFs(f.meta), nil
}

func (f *RegularFile) Read(bytes []byte) (int, error) {
	f.mu.RLock()
	r := f.curReader
	cursor := f.readCursor
	f.mu.RUnlock()

	if cursor >= int64(f.meta.Size) {
		return 0, io.EOF
	}

	var err error

	if r == nil {
		// No reader available, make new GetFile call.
		_, r, err = f.pfs.peer.GetFile(&pb.MsgGetFile{
			Path:   f.path.String(),
			Offset: uint64(cursor),
		})
		if err != nil {
			return 0, f.pfs.refineError(err)
		}
		f.mu.Lock()
		f.curReader = r
		f.mu.Unlock()
	}

	n, err := r.Read(bytes)
	f.mu.Lock()
	f.readCursor += int64(n)
	f.mu.Unlock()
	return n, err
}

var errSeekNegative = errors.New("seek offset cannot resolve to an index before the start of the file")

func (f *RegularFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.RLock()
	oldReader := f.curReader
	oldCursor := f.readCursor
	f.mu.Unlock()

	fsize := int64(f.meta.Size)

	var newCursor int64
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, errSeekNegative
		}

		newCursor = offset
	case io.SeekCurrent:
		if oldCursor+offset < 0 {
			return 0, errSeekNegative
		}

		newCursor = oldCursor + offset
	case io.SeekEnd:
		if fsize+offset < 0 {
			return 0, errSeekNegative
		}

		newCursor = fsize + offset
	default:
		return 0, fmt.Errorf(`unknown whence value %d`, whence)
	}

	if newCursor != oldCursor {
		f.mu.Lock()
		f.readCursor = newCursor
		if oldReader != nil {
			// New cursor is different from the old one, close the old reader if any.
			_ = oldReader.Close()
			f.curReader = nil
		}
		f.mu.Unlock()
	}

	return newCursor, nil
}

func (f *RegularFile) Close() error {
	f.mu.Lock()
	r := f.curReader
	f.mu.Unlock()

	if r != nil {
		return r.Close()
	}
	return nil
}

// DirFile represents a directory shared by a peer.
// It implements fs.File and fs.ReadDirFile, and it makes GetDirFiles calls to the peer under the hood.
type DirFile struct {
	mu sync.RWMutex

	pfs *PeerFs

	path common.ProtoPath
	meta *pb.MsgFileMeta
}

func NewPeerFsDirFile(pfs *PeerFs, path common.ProtoPath, meta *pb.MsgFileMeta) *DirFile {
	return &DirFile{
		pfs: pfs,

		path: path,
		meta: meta,
	}
}

var _ fs.File = (*DirFile)(nil)
var _ fs.ReadDirFile = (*DirFile)(nil)

func (f *DirFile) Stat() (fs.FileInfo, error) {
	return MetaToFs(f.meta), nil
}
func (f *DirFile) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf(`tried to get file content in peer %q path %q, but it was directory`, f.pfs.username.String(), f.path.String())
}

func (f *DirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	var limit int
	if n < 1 {
		n = 0
		limit = math.MaxInt
	} else {
		limit = n
	}

	stream, err := f.pfs.peer.GetDirFiles(f.path)
	if err != nil {
		return nil, f.pfs.refineError(err)
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

	cache := f.pfs.cacheOrNil
	keyPrefix := f.pfs.cachePrefix

	for {
		next, nextErr := stream.ReadNext()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			return nil, f.pfs.refineError(nextErr)
		}

		for _, file := range next.Files {
			if len(entries) < limit {
				entries = append(entries, MetaToFs(file))
			}

			if cache != nil {
				path := common.UncheckedCreateProtoPath(f.path.String() + "/" + file.Name)
				cache.Set(keyPrefix, path, file)
			}
		}
	}

	return entries, nil
}

func (f *DirFile) Close() error {
	return nil
}
