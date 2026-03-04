package fsys

import (
	"io/fs"
	"time"
)

// FsFilePerms are the permissions to expose for files in FriendNet fs.FS implementations.
//
// Owner: read-only
// Group: read-only
// Other: read-only
const FsFilePerms fs.FileMode = 0o444

// FsModTime is the modification time to expose for files in FriendNet fs.FS implementations.
//
// 1970/01/01 00:00:00 UTC
var FsModTime = time.Unix(0, 0)

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
	return FsModTime
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
