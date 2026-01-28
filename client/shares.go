package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pb "friendnet.org/protocol/pb/v1"
)

type ShareManager struct {
	shares     map[string]string
	shareNames []string
}

func NewShareManager(cfg *ClientConfig) *ShareManager {
	shares := make(map[string]string)
	names := make([]string, 0)
	if cfg != nil {
		for _, share := range cfg.Shares {
			if share.Path == "" {
				continue
			}
			name := share.Name
			if name == "" {
				name = filepath.Base(share.Path)
			}
			if name == "" {
				continue
			}
			names = append(names, name)
			shares[strings.ToLower(name)] = share.Path
		}
	}

	return &ShareManager{
		shares:     shares,
		shareNames: names,
	}
}

func (s *ShareManager) ResolvePath(requestPath string) (string, error) {
	if requestPath == "" || !strings.HasPrefix(requestPath, "/") {
		return "", fmt.Errorf("path must start with '/'")
	}

	parts := strings.Split(strings.TrimPrefix(requestPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", fmt.Errorf("path must include share name")
	}

	shareRoot, ok := s.shares[strings.ToLower(parts[0])]
	if !ok {
		return "", fmt.Errorf("unknown share %q", parts[0])
	}

	subPath := strings.Join(parts[1:], "/")
	joined := filepath.Join(shareRoot, filepath.FromSlash(subPath))

	rootAbs, err := filepath.Abs(shareRoot)
	if err != nil {
		return "", err
	}
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	if joinedAbs != rootAbs && !strings.HasPrefix(joinedAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes share root")
	}

	return joinedAbs, nil
}

func (s *ShareManager) ListDir(requestPath string) (*pb.MsgDirFiles, error) {
	if requestPath == "/" {
		names := make([]string, 0, len(s.shareNames))
		names = append(names, s.shareNames...)
		return &pb.MsgDirFiles{Filenames: names}, nil
	}

	realPath, err := s.ResolvePath(requestPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(realPath)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return &pb.MsgDirFiles{Filenames: names}, nil
}

func (s *ShareManager) FileMeta(requestPath string) (*pb.MsgFileMeta, *os.File, error) {
	realPath, err := s.ResolvePath(requestPath)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(realPath)
	if err != nil {
		return nil, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return &pb.MsgFileMeta{Size: 0, IsDir: true}, nil, nil
	}

	return &pb.MsgFileMeta{Size: uint64(info.Size()), IsDir: false}, file, nil
}

func (s *ShareManager) CopyFile(requestPath string, offset uint64, limit uint64, w io.Writer) (*pb.MsgFileMeta, error) {
	meta, file, err := s.FileMeta(requestPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	if offset > 0 {
		if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
			return nil, err
		}
	}

	if limit > 0 {
		if _, err := io.CopyN(w, file, int64(limit)); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	} else {
		if _, err := io.Copy(w, file); err != nil {
			return nil, err
		}
	}

	return meta, nil
}
