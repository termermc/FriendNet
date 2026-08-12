package share

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/fsnotify/fsnotify"
)

// ErrShareClosed is returned by Share methods when the share is closed.
var ErrShareClosed = errors.New("share closed")

type ShareCallback func(path common.ProtoPath)

// Share is a shared filesystem.
// A share only has the concepts of files and directories.
// It has no way of representing symlinks or pipes.
// It is up to the implementation on how to represent or ignore these concepts.
//
// The Close method may be no-op for some implementations.
type Share interface {
	io.Closer

	// Name returns the name of the share.
	Name() string

	// GetFileMeta returns the metadata for a path.
	// The path may be a file or a directory.
	// Must be able to handle a request for "/".
	//
	// Returns fs.ErrNotExist if the path does not exist.
	// Returns fs.ErrPermission if access is denied.
	//
	// May return ErrShareClosed if the share is closed, depending on the implementation.
	GetFileMeta(path common.ProtoPath) (*pb.MsgFileMeta, error)

	// DirFiles returns metadata for all files in the directory at the specified path.
	// Must be able to handle a request for "/".
	//
	// Returns fs.ErrNotExist if the path does not exist.
	// Returns fs.ErrPermission if access is denied.
	//
	// May return ErrShareClosed if the share is closed, depending on the implementation.
	DirFiles(path common.ProtoPath) ([]*pb.MsgFileMeta, error)

	// GetFile returns the metadata for a path and a stream of its binary content (if not a directory).
	// Important: If the file is a directory, the stream will be empty and always return io.EOF.
	//
	// `offset` is the offset into the file to read, in bytes.
	// Values above the file size will just result in no data being returned.
	//
	// `limit` is the limit of the file to read, in bytes.
	// Specify 0 for no limit.
	//
	// Returns fs.ErrNotExist if the path does not exist.
	// Returns fs.ErrPermission if access is denied.
	//
	// May return ErrShareClosed if the share is closed, depending on the implementation.
	GetFile(path common.ProtoPath, offset uint64, limit uint64) (*pb.MsgFileMeta, io.ReadCloser, error)

	// SupportsWatching will return true if the Share implementation supports filesystem event watching.
	SupportsWatching() bool

	// OnNeedIndex subscribes a callback to a filesystem event listener.
	// The callbacks will fire, in order of subscription, when a new file in a watched directory is created or if an existing file has been modified.
	OnNeedIndex(callback ShareCallback)

	// OnDelete subscribes a callback to a filesystem event listener.
	// The callbacks will fire, in order of subscription, when a file in a watched directory is deleted.
	OnDelete(callback ShareCallback)
}

// DirShare is an implementation of Share backed by a directory.
type DirShare struct {
	ctx context.Context

	name        string
	dir         string
	followLinks bool
	fsys        fs.FS

	// Watching related members
	mu sync.RWMutex

	watcher       *fsnotify.Watcher
	onIndexHdlrs  []ShareCallback
	onDeleteHdlrs []ShareCallback
}

var _ Share = (*DirShare)(nil)

func (s *DirShare) Close() error {
	return s.watcher.Close()
}

// NewDirShare creates a new DirShare backed by the specified directory.
// It will also initialize a filesystem watcher.
// If followLinks is false, symlinks will be treated as if they do not exist.
func NewDirShare(
	ctx context.Context,
	name string,
	dir string,
	followLinks bool,
) (*DirShare, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	// Setup watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	share := &DirShare{
		ctx:         ctx,
		name:        name,
		dir:         abs,
		followLinks: followLinks,
		fsys:        os.DirFS(abs),
		watcher:     watcher,
	}

	err = watcher.Add(abs)
	if err != nil {
		return nil, err
	}

	// Init watcher
	// On errors, just kill the watcher
	go func() {
		var (
			dedupDelay   = 100 * time.Millisecond
			dedupTimerMu sync.Mutex
			dedupTimers  = make(map[string]*time.Timer)
		)

		// Crawl for subdirectories to add to watcher
		filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				_ = watcher.Add(path)
			}

			return nil
		})

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					break
				}

				if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) && !event.Has(fsnotify.Chmod) {
					continue
				}

				share.mu.RLock()

				if len(event.Name) == 0 {
					continue
				}

				dedupTimerMu.Lock()
				t, ok := dedupTimers[event.Name]
				dedupTimerMu.Unlock()

				// If timer for item doesn't exist, create
				if !ok {
					t = time.AfterFunc(math.MaxInt64, func() {
						evtPath := event.Name

						// If this is a directory, add it to the watches.
						stat, err := os.Stat(evtPath)
						if err == nil && stat.IsDir() {
							_ = watcher.Add(evtPath)
						}

						relPath, err := filepath.Rel(abs, evtPath)
						if err != nil {
							return
						}

						fmt.Printf("event: %s %s\n", event.Op.String(), relPath)

						path, err := common.NormalizePath(relPath)
						if err != nil {
							return
						}

						if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
							for _, cb := range share.onIndexHdlrs {
								cb(path)
							}
						} else if event.Has(fsnotify.Remove) {
							for _, cb := range share.onDeleteHdlrs {
								cb(path)
							}
						}

						share.mu.RUnlock()
					})

					t.Stop()

					dedupTimerMu.Lock()
					dedupTimers[event.Name] = t
					dedupTimerMu.Unlock()
				}

				t.Reset(dedupDelay)
			}
		}
	}()

	return share, nil
}

func (s *DirShare) SupportsWatching() bool {
	return true
}

func (s *DirShare) OnNeedIndex(callback ShareCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.onIndexHdlrs = append(s.onIndexHdlrs, callback)
}

func (s *DirShare) OnDelete(callback ShareCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.onDeleteHdlrs = append(s.onDeleteHdlrs, callback)
}

func (s *DirShare) isInfoOk(info fs.FileInfo) bool {
	if s.followLinks {
		return true
	}

	mode := info.Mode()
	return mode.IsRegular() || mode.IsDir()
}
func (s *DirShare) stat(path common.ProtoPath) (fs.FileInfo, error) {
	var stat fs.FileInfo
	var err error
	if path.IsRoot() {
		if s.followLinks {
			stat, err = os.Stat(s.dir)
		} else {
			stat, err = os.Lstat(s.dir)
		}
	} else {
		if s.followLinks {
			stat, err = fs.Stat(s.fsys, path.String()[1:])
		} else {
			stat, err = fs.Lstat(s.fsys, path.String()[1:])
		}
	}
	if err != nil {
		// fs.Stat already returns errors compatible with fs.ErrNotExist and fs.ErrPermission.
		return nil, err
	}

	if !s.isInfoOk(stat) {
		return nil, fs.ErrNotExist
	}

	return stat, nil
}
func (s *DirShare) pathOk(path common.ProtoPath) bool {
	if s.followLinks {
		return true
	}
	if path.IsRoot() {
		return true
	}

	// Symlinks are now allowed.
	// Go through containing directories and check if any of them are symlinks.

	segments := path.ToSegments()

	for i := 0; i < len(segments); i++ {
		stat, err := s.stat(common.UncheckedCreateProtoPath("/" + strings.Join(segments[:i+1], "/")))
		if err != nil {
			return false
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}

	return true
}

func (s *DirShare) Name() string {
	return s.name
}

func (s *DirShare) GetFileMeta(path common.ProtoPath) (*pb.MsgFileMeta, error) {
	if path.IsRoot() {
		return &pb.MsgFileMeta{
			Name:  "/",
			IsDir: true,
			Size:  0,
		}, nil
	}

	if !s.pathOk(path) {
		return nil, fs.ErrNotExist
	}

	info, err := s.stat(path)
	if err != nil {
		return nil, err
	}

	return fileInfoToMeta(info), nil
}

func (s *DirShare) DirFiles(path common.ProtoPath) ([]*pb.MsgFileMeta, error) {
	if !s.pathOk(path) {
		return nil, fs.ErrNotExist
	}

	var entries []fs.DirEntry
	var readDirErr error
	if path.IsRoot() {
		// DirFS does not support ReadDir on "/", so we do it directly on the directory path.
		entries, readDirErr = os.ReadDir(s.dir)
	} else {
		entries, readDirErr = fs.ReadDir(s.fsys, path.String()[1:])
	}
	if readDirErr != nil {
		return nil, readDirErr
	}

	out := make([]*pb.MsgFileMeta, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}

		if s.followLinks && info.Mode()&os.ModeSymlink != 0 {
			var statPath string
			if path.IsRoot() {
				statPath = "/" + entry.Name()
			} else {
				statPath = path.String() + "/" + entry.Name()
			}
			info, err = s.stat(common.UncheckedCreateProtoPath(statPath))
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, err
			}
		}

		if !s.isInfoOk(info) {
			continue
		}
		out = append(out, fileInfoToMeta(info))
	}

	return out, nil
}

func (s *DirShare) GetFile(
	path common.ProtoPath,
	offset uint64,
	limit uint64,
) (*pb.MsgFileMeta, io.ReadCloser, error) {
	if path.IsRoot() {
		return &pb.MsgFileMeta{
			Name:  "/",
			IsDir: true,
			Size:  0,
		}, common.EofReadCloser{}, nil
	}

	if !s.pathOk(path) {
		return nil, nil, fs.ErrNotExist
	}

	info, err := s.stat(path)
	if err != nil {
		return nil, nil, err
	}

	meta := fileInfoToMeta(info)

	if meta.IsDir {
		// Directory; nothing to read.
		return meta, common.EofReadCloser{}, nil
	}
	if offset >= meta.Size {
		// Offset >= file size; nothing to read.
		return meta, common.EofReadCloser{}, nil
	}

	f, err := s.fsys.Open(path.String()[1:])
	if err != nil {
		return nil, nil, err
	}

	// Close if we weren't able to open and seek.
	openOk := false
	defer func() {
		if !openOk {
			_ = f.Close()
		}
	}()

	// We have two options:
	//  - Seek if the underlying type is io.Seeker
	//  - Fall back to emulating seeking by discarding offset (expensive)
	var rc io.ReadCloser = f
	if offset > 0 {
		if seeker, ok := f.(io.Seeker); ok {
			if _, err = seeker.Seek(int64(offset), io.SeekStart); err != nil {
				return nil, nil, err
			}
		} else {
			if _, err := io.CopyN(io.Discard, f, int64(offset)); err != nil {
				// If offset is past EOF, CopyN returns io.EOF; treat as empty stream.
				if !errors.Is(err, io.EOF) {
					return nil, nil, err
				}
			}
		}
	}

	openOk = true

	if limit > 0 {
		rc = common.NewLimitReadCloser(f, int64(limit))
	}

	return meta, rc, nil
}

func fileInfoToMeta(info fs.FileInfo) *pb.MsgFileMeta {
	isDir := info.IsDir()

	var size uint64
	if !isDir {
		// I don't even know how a file could have a negative size, but we'll just use 0 if it does.
		if info.Size() > 0 {
			size = uint64(info.Size())
		} else {
			size = 0
		}
	}

	return &pb.MsgFileMeta{
		Name:  info.Name(),
		IsDir: isDir,
		Size:  size,
	}
}
