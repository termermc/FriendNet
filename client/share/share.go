package share

import (
	"errors"
	"io"
	"io/fs"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// ErrShareClosed is returned by Share methods when the share is closed.
var ErrShareClosed = errors.New("share closed")

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
	//
	// Returns fs.ErrNotExist if the path does not exist.
	// Returns fs.ErrPermission if access is denied.
	//
	// May return ErrShareClosed if the share is closed, depending on the implementation.
	GetFileMeta(path protocol.ProtoPath) (*pb.MsgFileMeta, error)

	// DirFiles returns metadata for all files in the directory at the specified path.
	//
	// Returns fs.ErrNotExist if the path does not exist.
	// Returns fs.ErrPermission if access is denied.
	//
	// May return ErrShareClosed if the share is closed, depending on the implementation.
	DirFiles(path protocol.ProtoPath) ([]*pb.MsgFileMeta, error)

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
	GetFile(path protocol.ProtoPath, offset uint64, limit uint64) (*pb.MsgFileMeta, io.ReadCloser, error)
}

// FsShare is an implementation of Share backed by an fs.FS instance.
type FsShare struct {
	name string
	fsys fs.FS
}

var _ Share = (*FsShare)(nil)

// Close is no-op because FsShare is stateless.
func (s *FsShare) Close() error {
	return nil
}

// NewFsShare creates a new FsShare backed by the specified fs.FS instance.
func NewFsShare(name string, fsys fs.FS) *FsShare {
	return &FsShare{
		name: name,
		fsys: fsys,
	}
}

func (s *FsShare) Name() string {
	return s.name
}

func (s *FsShare) GetFileMeta(path protocol.ProtoPath) (*pb.MsgFileMeta, error) {
	info, err := fs.Stat(s.fsys, path.String())
	if err != nil {
		// fs.Stat already returns errors compatible with fs.ErrNotExist and fs.ErrPermission.
		return nil, err
	}

	return fileInfoToMeta(info), nil
}

func (s *FsShare) DirFiles(path protocol.ProtoPath) ([]*pb.MsgFileMeta, error) {
	entries, readDirErr := fs.ReadDir(s.fsys, path.String())
	if readDirErr != nil {
		return nil, readDirErr
	}

	out := make([]*pb.MsgFileMeta, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, fileInfoToMeta(info))
	}

	return out, nil
}

func (s *FsShare) GetFile(
	path protocol.ProtoPath,
	offset uint64,
	limit uint64,
) (*pb.MsgFileMeta, io.ReadCloser, error) {
	info, err := fs.Stat(s.fsys, path.String())
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

	f, err := s.fsys.Open(path.String())
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
