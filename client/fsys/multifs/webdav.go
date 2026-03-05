package multifs

import (
	"context"
	"os"

	"golang.org/x/net/webdav"
)

// WebDavWrapper wraps MultiFs to implement webdav.FileSystem.
// It is read-only, and all write operations return os.ErrPermission.
type WebDavWrapper struct {
	mfs *MultiFs
}

func NewWebDavWrapper(mfs *MultiFs) WebDavWrapper {
	return WebDavWrapper{
		mfs: mfs,
	}
}

func (w WebDavWrapper) Mkdir(_ context.Context, _ string, _ os.FileMode) error {
	return os.ErrPermission
}

func (w WebDavWrapper) OpenFile(_ context.Context, name string, _ int, _ os.FileMode) (webdav.File, error) {
	f, err := w.mfs.Open(name)
	if err != nil {
		return nil, err
	}

	return f.(webdav.File), nil
}

func (w WebDavWrapper) RemoveAll(_ context.Context, _ string) error {
	return os.ErrPermission
}

func (w WebDavWrapper) Rename(_ context.Context, _ string, _ string) error {
	return os.ErrPermission
}

func (w WebDavWrapper) Stat(_ context.Context, name string) (os.FileInfo, error) {
	return w.mfs.Stat(name)
}

var _ webdav.FileSystem = (*WebDavWrapper)(nil)
