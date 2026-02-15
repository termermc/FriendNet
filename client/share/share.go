package share

import (
	"io"

	pb "friendnet.org/protocol/pb/v1"
)

// Share is a shared filesystem.
// A share only has the concepts of files and directories.
// It has no way of representing symlinks or pipes.
// It is up to the implementation on how to represent or ignore these concepts.
type Share interface {
	// GetFileMeta returns the metadata for a path.
	// The path may be a file or a directory.
	// Returns fs.ErrNotExist if the path does not exist.
	GetFileMeta(path string) (*pb.MsgFileMeta, error)

	// DirFiles returns metadata for all files in the directory at the specified path.
	// Returns fs.ErrNotExist if the path does not exist.
	DirFiles(path string) ([]*pb.MsgFileMeta, error)

	// GetFile returns the metadata for a path and a stream of its binary content (if not a directory).
	// Important: If the file is a directory, the stream will be nil.
	GetFile(path string) (*pb.MsgFileMeta, io.ReadCloser, error)
}

// DirShare is an implementation of Share backed by a local directory.
// TODO Use DirFS?
type DirShare struct {
}
