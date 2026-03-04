package peerfs

import (
	"context"
	"io/fs"

	"friendnet.org/client"
	"friendnet.org/client/room"
	"friendnet.org/common"
)

// NannyFs wraps peerfs.PeerFs with a client.ConnNanny, waiting for an open connection before performing operations.
// Just like PeerFs, it is stateless.
// It is suitable to use for single operations and then throwing it away.
//
// Closing NannyFs is a no-op and does not close any resources it uses.
type NannyFs struct {
	ctx context.Context

	cn *client.ConnNanny

	username common.NormalizedUsername
	opts     []Option
}

// NewNannyFs creates a new NannyFs with the specified ConnNanny, peer username, and PeerFs options.
//
// The context passed to it will be used for timing out waits for an open connection.
//
// Keep in mind that opened files are from PeerFs, and so if the connection they were opened with is
// severed, they will break as if we were not using NannyFs.
// NannyFs is mainly a layer to wait for an open connection before calling PeerFs.Open.
func NewNannyFs(ctx context.Context, cn *client.ConnNanny, username common.NormalizedUsername, opts ...Option) *NannyFs {
	if ctx == nil {
		ctx = context.Background()
	}

	pfs := &NannyFs{
		ctx: ctx,

		cn: cn,

		username: username,
		opts:     opts,
	}

	return pfs
}

var _ fs.FS = (*NannyFs)(nil)
var _ fs.StatFS = (*NannyFs)(nil)
var _ fs.ReadFileFS = (*NannyFs)(nil)
var _ fs.ReadDirFS = (*NannyFs)(nil)

func (f *NannyFs) do(fn func(pfs *PeerFs) error) error {
	return f.cn.Do(f.ctx, func(_ context.Context, c *room.Conn) error {
		return fn(NewPeerFs(c, f.username, f.opts...))
	})
}

func doVal[T any](f *NannyFs, fn func(pfs *PeerFs) (T, error)) (T, error) {
	var res T
	doErr := f.do(func(pfs *PeerFs) error {
		val, err := fn(pfs)
		if err != nil {
			return err
		}
		res = val
		return nil
	})
	return res, doErr
}

// Open returns a DirFile or RegularFile for the specified path.
// Its error return value does not need to be refined.
func (f *NannyFs) Open(name string) (fs.File, error) {
	return doVal[fs.File](f, func(pfs *PeerFs) (fs.File, error) {
		return pfs.Open(name)
	})
}

func (f *NannyFs) Stat(name string) (fs.FileInfo, error) {
	return doVal[fs.FileInfo](f, func(pfs *PeerFs) (fs.FileInfo, error) {
		return pfs.Stat(name)
	})
}

func (f *NannyFs) ReadFile(name string) ([]byte, error) {
	return doVal[[]byte](f, func(pfs *PeerFs) ([]byte, error) {
		return pfs.ReadFile(name)
	})
}

func (f *NannyFs) ReadDir(name string) ([]fs.DirEntry, error) {
	return doVal[[]fs.DirEntry](f, func(pfs *PeerFs) ([]fs.DirEntry, error) {
		return pfs.ReadDir(name)
	})
}
