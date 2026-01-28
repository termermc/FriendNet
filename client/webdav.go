package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	pb "friendnet.org/protocol/pb/v1"
	"golang.org/x/net/webdav"
)

type webdavClient interface {
	GetOnlineUsers() ([]string, error)
	GetDirFiles(user string, path string) ([]string, error)
	GetFileMeta(user string, path string) (*pb.MsgFileMeta, error)
	GetFile(user string, path string, offset uint64, limit uint64) (*pb.MsgFileMeta, io.ReadCloser, error)
}

type WebDAVServer struct {
	port   int
	getter func() webdavClient
	mu     sync.Mutex
	server *http.Server
	ln     net.Listener
}

func NewWebDAVServer(port int, getter func() webdavClient) *WebDAVServer {
	return &WebDAVServer{
		port:   port,
		getter: getter,
	}
}

func (s *WebDAVServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return fmt.Errorf("webdav already started")
	}

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	fs := &proxyFS{getClient: s.getter}
	handler := &webdav.Handler{
		Prefix:     "/",
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}

	server := &http.Server{
		Handler:  handler,
		ErrorLog: log.New(io.Discard, "", 0),
	}

	s.server = server
	s.ln = ln

	go func() {
		_ = server.Serve(ln)
	}()

	return nil
}

func (s *WebDAVServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	err := s.server.Shutdown(ctx)
	s.server = nil
	if s.ln != nil {
		_ = s.ln.Close()
		s.ln = nil
	}
	return err
}

type proxyFS struct {
	getClient func() webdavClient
}

func (p *proxyFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, os.ErrPermission
	}

	client := p.getClient()
	if client == nil {
		return nil, errors.New("not connected")
	}

	clean := path.Clean("/" + strings.TrimPrefix(name, "/"))
	if clean == "/" {
		users, err := client.GetOnlineUsers()
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(users))
		for _, user := range users {
			infos = append(infos, fileInfo{name: user, mode: os.ModeDir | 0o755, isDir: true})
		}
		return &dirFile{infos: infos}, nil
	}

	user, subPath := splitUserPath(clean)
	if user == "" {
		return nil, os.ErrNotExist
	}

	if subPath == "" || subPath == "/" {
		subPath = "/"
		files, err := client.GetDirFiles(user, subPath)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(files))
		for _, name := range files {
			entryPath := joinDirPath(subPath, name)
			entryMeta, err := client.GetFileMeta(user, entryPath)
			if err != nil {
				infos = append(infos, fileInfo{name: name, mode: 0o644})
				continue
			}
			if entryMeta.IsDir {
				infos = append(infos, fileInfo{name: name, mode: os.ModeDir | 0o755, isDir: true})
			} else {
				infos = append(infos, fileInfo{name: name, size: int64(entryMeta.Size), mode: 0o644})
			}
		}
		return &dirFile{infos: infos}, nil
	}

	meta, err := client.GetFileMeta(user, subPath)
	if err == nil && meta.IsDir {
		files, err := client.GetDirFiles(user, subPath)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(files))
		for _, name := range files {
			entryPath := joinDirPath(subPath, name)
			entryMeta, err := client.GetFileMeta(user, entryPath)
			if err != nil {
				infos = append(infos, fileInfo{name: name, mode: 0o644})
				continue
			}
			if entryMeta.IsDir {
				infos = append(infos, fileInfo{name: name, mode: os.ModeDir | 0o755, isDir: true})
			} else {
				infos = append(infos, fileInfo{name: name, size: int64(entryMeta.Size), mode: 0o644})
			}
		}
		return &dirFile{infos: infos}, nil
	}

	if err != nil {
		return nil, err
	}
	if meta.IsDir {
		return nil, os.ErrNotExist
	}

	return &streamFile{
		client: client,
		user:   user,
		path:   subPath,
		size:   int64(meta.Size),
		info:   fileInfo{name: path.Base(subPath), size: int64(meta.Size), mode: 0o644},
	}, nil
}

func (p *proxyFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (p *proxyFS) RemoveAll(ctx context.Context, name string) error {
	return os.ErrPermission
}

func (p *proxyFS) Rename(ctx context.Context, oldName, newName string) error {
	return os.ErrPermission
}

func (p *proxyFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	client := p.getClient()
	if client == nil {
		return nil, errors.New("not connected")
	}

	clean := path.Clean("/" + strings.TrimPrefix(name, "/"))
	if clean == "/" {
		return fileInfo{name: "/", mode: os.ModeDir | 0o755, isDir: true}, nil
	}

	user, subPath := splitUserPath(clean)
	if user == "" {
		return nil, os.ErrNotExist
	}
	if subPath == "" || subPath == "/" {
		return fileInfo{name: user, mode: os.ModeDir | 0o755, isDir: true}, nil
	}

	meta, err := client.GetFileMeta(user, subPath)
	if err != nil {
		return nil, err
	}
	if meta.IsDir {
		return fileInfo{name: path.Base(subPath), mode: os.ModeDir | 0o755, isDir: true}, nil
	}
	return fileInfo{name: path.Base(subPath), size: int64(meta.Size), mode: 0o644}, nil
}

func splitUserPath(cleanPath string) (string, string) {
	clean := strings.TrimPrefix(cleanPath, "/")
	if clean == "" {
		return "", ""
	}
	parts := strings.SplitN(clean, "/", 2)
	user := parts[0]
	subPath := "/"
	if len(parts) == 2 && parts[1] != "" {
		subPath = "/" + parts[1]
	}
	return user, subPath
}

func joinDirPath(base string, name string) string {
	if base == "/" {
		return "/" + name
	}
	return base + "/" + name
}

type dirFile struct {
	infos []os.FileInfo
	pos   int
}

func (d *dirFile) Close() error { return nil }

func (d *dirFile) Read([]byte) (int, error) { return 0, io.EOF }

func (d *dirFile) Write([]byte) (int, error) { return 0, os.ErrPermission }

func (d *dirFile) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == io.SeekStart {
		d.pos = 0
		return 0, nil
	}
	return 0, errors.New("seek not supported on directory")
}

func (d *dirFile) Readdir(count int) ([]os.FileInfo, error) {
	if d.pos >= len(d.infos) && count > 0 {
		return nil, io.EOF
	}

	if count <= 0 {
		return d.infos, nil
	}

	end := d.pos + count
	if end > len(d.infos) {
		end = len(d.infos)
	}
	result := d.infos[d.pos:end]
	d.pos = end
	return result, nil
}

func (d *dirFile) Stat() (os.FileInfo, error) {
	return fileInfo{name: "/", mode: os.ModeDir | 0o755, isDir: true}, nil
}

type tempFile struct {
	*os.File
	info os.FileInfo
}

func (t *tempFile) Stat() (os.FileInfo, error) {
	return t.info, nil
}

func (t *tempFile) Close() error {
	name := t.File.Name()
	err := t.File.Close()
	_ = os.Remove(name)
	return err
}

type streamFile struct {
	client webdavClient
	user   string
	path   string
	size   int64
	info   os.FileInfo
	reader io.ReadCloser
	offset int64
}

func (s *streamFile) Close() error {
	if s.reader != nil {
		_ = s.reader.Close()
		s.reader = nil
	}
	return nil
}

func (s *streamFile) Read(p []byte) (int, error) {
	if s.reader == nil {
		_, reader, err := s.client.GetFile(s.user, s.path, uint64(s.offset), 0)
		if err != nil {
			return 0, err
		}
		s.reader = reader
	}

	n, err := s.reader.Read(p)
	if n > 0 {
		s.offset += int64(n)
	}
	if errors.Is(err, io.EOF) {
		_ = s.reader.Close()
		s.reader = nil
	}
	return n, err
}

func (s *streamFile) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = s.offset + offset
	case io.SeekEnd:
		next = s.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if next < 0 {
		return 0, errors.New("negative position")
	}
	s.offset = next
	if s.reader != nil {
		_ = s.reader.Close()
		s.reader = nil
	}
	return s.offset, nil
}

func (s *streamFile) Write([]byte) (int, error) { return 0, os.ErrPermission }

func (s *streamFile) Readdir(int) ([]os.FileInfo, error) { return nil, errors.New("not a directory") }

func (s *streamFile) Stat() (os.FileInfo, error) { return s.info, nil }

type fileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (f fileInfo) Name() string       { return f.name }
func (f fileInfo) Size() int64        { return f.size }
func (f fileInfo) Mode() os.FileMode  { return f.mode }
func (f fileInfo) ModTime() time.Time { return time.Time{} }
func (f fileInfo) IsDir() bool        { return f.isDir }
func (f fileInfo) Sys() any           { return nil }
