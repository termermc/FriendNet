package fsys

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/webdav"
)

// FsFilePerms are the permissions to expose for files in FriendNet fs.FS implementations.
//
// Owner: read-only
// Group: read-only
// Other: read-only
const FsFilePerms fs.FileMode = 0o444

// DummyDirFsWrapper is a directory name that implements fs.FileInfo, and fs.DirEntry.
// The name must be a valid filename, so it must not contain any slashes.
type DummyDirFsWrapper string

var _ fs.FileInfo = DummyDirFsWrapper("")
var _ fs.DirEntry = DummyDirFsWrapper("")

func (w DummyDirFsWrapper) Name() string {
	return string(w)
}
func (w DummyDirFsWrapper) Size() int64 {
	return 0
}
func (w DummyDirFsWrapper) Mode() fs.FileMode {
	return fs.ModeDir | FsFilePerms
}
func (w DummyDirFsWrapper) ModTime() time.Time {
	return time.Now()
}
func (w DummyDirFsWrapper) IsDir() bool {
	return true
}
func (w DummyDirFsWrapper) Sys() any {
	return nil
}
func (w DummyDirFsWrapper) Type() fs.FileMode {
	return fs.ModeDir
}
func (w DummyDirFsWrapper) Info() (fs.FileInfo, error) {
	return w, nil
}

// DummySeek is a dummy implementation of io.Seeker's Seek.
// It behaves the same way os.File's Seek behaves when the file is a directory.
func DummySeek(offset int64, whence int) (int64, error) {
	var ret int64
	switch whence {
	case io.SeekCurrent:
		fallthrough
	case io.SeekStart:
		if offset < 0 {
			return 0, errors.New("invalid offset")
		}
		ret = offset
	case io.SeekEnd:
		return 0, errors.New("seek from end not supported")
	}

	return ret, nil
}

// DirWithChildrenFile is a file that acts as a directory containing a predetermined set of children.
type DirWithChildrenFile struct {
	mu sync.Mutex

	name      string
	children  []fs.DirEntry
	dirCursor int
}

var _ fs.File = (*DirWithChildrenFile)(nil)
var _ io.Seeker = (*DirWithChildrenFile)(nil)
var _ fs.ReadDirFile = (*DirWithChildrenFile)(nil)
var _ http.File = (*DirWithChildrenFile)(nil)
var _ webdav.File = (*DirWithChildrenFile)(nil)

// NewDirWithChildrenFile creates a new DirWithChildrenFile.
func NewDirWithChildrenFile(name string, children []fs.DirEntry) *DirWithChildrenFile {
	return &DirWithChildrenFile{
		name:      name,
		children:  children,
		dirCursor: 0,
	}
}

func (f *DirWithChildrenFile) Stat() (fs.FileInfo, error) {
	return DummyDirFsWrapper(f.name), nil
}

func (f *DirWithChildrenFile) Read(_ []byte) (n int, err error) {
	return 0, fmt.Errorf(`file %q is a directory, it cannot be read`, f.name)
}

func (f *DirWithChildrenFile) ReadDir(n int) ([]fs.DirEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.dirCursor >= len(f.children) {
		if n > 0 {
			return nil, io.EOF
		}

		return nil, nil
	}

	if n == 0 {
		res := f.children[f.dirCursor:]
		f.dirCursor = len(f.children)
		return res, nil
	}

	endIdx := min(n, len(f.children))
	res := f.children[f.dirCursor:endIdx]
	f.dirCursor = endIdx

	if endIdx == len(f.children) {
		return res, io.EOF
	}

	return res, nil
}

func (f *DirWithChildrenFile) Readdir(count int) ([]fs.FileInfo, error) {
	entries, err := f.ReadDir(count)
	infos := make([]fs.FileInfo, len(entries))
	for i, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infos[:i], infoErr
		}
		infos[i] = info
	}
	return infos, err
}

func (f *DirWithChildrenFile) Seek(offset int64, whence int) (int64, error) {
	return DummySeek(offset, whence)
}

func (f *DirWithChildrenFile) Write(_ []byte) (n int, err error) {
	return 0, fmt.Errorf(`file %q is a directory, it cannot be written to`, f.name)
}

func (f *DirWithChildrenFile) Close() error {
	return nil
}
