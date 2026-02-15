package share

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	pb "friendnet.org/protocol/pb/v1"
)

const shareName = "testshare"

func TestFsShare_GetFileMeta_File(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"dir/hello.txt": &fstest.MapFile{Data: []byte("hello")},
	})

	meta, err := s.GetFileMeta("dir/hello.txt")
	if err != nil {
		t.Fatalf("GetFileMeta error: %v", err)
	}
	if meta.Name != "hello.txt" {
		t.Fatalf("Name: got %q want %q", meta.Name, "hello.txt")
	}
	if meta.IsDir {
		t.Fatalf("IsDir: got true want false")
	}
	if meta.Size != 5 {
		t.Fatalf("Size: got %d want %d", meta.Size, 5)
	}
}

func TestFsShare_GetFileMeta_Dir(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"dir/hello.txt": &fstest.MapFile{Data: []byte("hello")},
	})

	meta, err := s.GetFileMeta("dir")
	if err != nil {
		t.Fatalf("GetFileMeta error: %v", err)
	}
	if meta.Name != "dir" {
		t.Fatalf("Name: got %q want %q", meta.Name, "dir")
	}
	if !meta.IsDir {
		t.Fatalf("IsDir: got false want true")
	}
	if meta.Size != 0 {
		t.Fatalf("Size: got %d want %d", meta.Size, 0)
	}
}

func TestFsShare_GetFileMeta_NotExist(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{})

	_, err := s.GetFileMeta("nope.txt")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist; got %T %v", err, err)
	}
}

func TestFsShare_DirFiles_Basic(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"dir/a.txt": &fstest.MapFile{Data: []byte("a")},
		"dir/b.bin": &fstest.MapFile{Data: []byte{1, 2, 3, 4}},
		"dir/sub/c": &fstest.MapFile{Data: []byte("c")},
	})

	metas, err := s.DirFiles("dir")
	if err != nil {
		t.Fatalf("DirFiles error: %v", err)
	}

	got := map[string]*pb.MsgFileMeta{}
	for _, m := range metas {
		got[m.Name] = m
	}

	if got["a.txt"] == nil || got["a.txt"].IsDir || got["a.txt"].Size != 1 {
		t.Fatalf("bad meta for a.txt: %#v", got["a.txt"])
	}
	if got["b.bin"] == nil || got["b.bin"].IsDir || got["b.bin"].Size != 4 {
		t.Fatalf("bad meta for b.bin: %#v", got["b.bin"])
	}
	if got["sub"] == nil || !got["sub"].IsDir || got["sub"].Size != 0 {
		t.Fatalf("bad meta for sub: %#v", got["sub"])
	}
}

func TestFsShare_DirFiles_NotExist(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"dir/a.txt": &fstest.MapFile{Data: []byte("a")},
	})

	_, err := s.DirFiles("missing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist; got %T %v", err, err)
	}
}

func TestFsShare_GetFile_DirReturnsEmptyStream(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"dir/a.txt": &fstest.MapFile{Data: []byte("a")},
	})

	meta, rc, err := s.GetFile("dir", 0, 0)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if meta == nil || !meta.IsDir {
		t.Fatalf("expected directory meta; got %#v", meta)
	}

	defer func() {
		_ = rc.Close()
	}()

	buf := make([]byte, 1024)
	n, err := rc.Read(buf)
	if n != 0 {
		t.Fatalf("expected empty stream; got at least %d bytes", n)
	}
	if err == nil {
		t.Fatalf("expected EOF, got no error")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF; got %T %v", err, err)
	}
}

func TestFsShare_GetFile_WholeFile(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"f.txt": &fstest.MapFile{Data: []byte("abcdef")},
	})

	meta, rc, err := s.GetFile("f.txt", 0, 0)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if meta.IsDir {
		t.Fatalf("expected file, got dir meta")
	}
	if meta.Size != 6 {
		t.Fatalf("Size: got %d want %d", meta.Size, 6)
	}
	if rc == nil {
		t.Fatalf("expected non-nil reader")
	}
	defer func() {
		_ = rc.Close()
	}()

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(b) != "abcdef" {
		t.Fatalf("content: got %q want %q", string(b), "abcdef")
	}
}

func TestFsShare_GetFile_Offset(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"f.txt": &fstest.MapFile{Data: []byte("abcdef")},
	})

	_, rc, err := s.GetFile("f.txt", 2, 0)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	defer func() {
		_ = rc.Close()
	}()

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(b) != "cdef" {
		t.Fatalf("content: got %q want %q", string(b), "cdef")
	}
}

func TestFsShare_GetFile_Limit(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"f.txt": &fstest.MapFile{Data: []byte("abcdef")},
	})

	_, rc, err := s.GetFile("f.txt", 0, 3)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	defer func() {
		_ = rc.Close()
	}()

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(b) != "abc" {
		t.Fatalf("content: got %q want %q", string(b), "abc")
	}
}

func TestFsShare_GetFile_OffsetAndLimit(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"f.txt": &fstest.MapFile{Data: []byte("abcdef")},
	})

	_, rc, err := s.GetFile("f.txt", 2, 3)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	defer func() {
		_ = rc.Close()
	}()

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(b) != "cde" {
		t.Fatalf("content: got %q want %q", string(b), "cde")
	}
}

func TestFsShare_GetFile_OffsetPastEOF(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"f.txt": &fstest.MapFile{Data: []byte("abc")},
	})

	_, rc, err := s.GetFile("f.txt", 999, 0)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	defer func() {
		_ = rc.Close()
	}()

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if len(b) != 0 {
		t.Fatalf("expected empty content; got %q", string(b))
	}
}

func TestFsShare_GetFile_NotExist(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{})

	_, _, err := s.GetFile("nope", 0, 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist; got %T %v", err, err)
	}
}

// ---- Extra coverage: non-seekable fs.File path ----

type nonSeekFS struct {
	fsys fs.FS
}

func (n nonSeekFS) Open(name string) (fs.File, error) {
	f, err := n.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	// Wrap to remove io.Seeker, while still being a fs.File.
	return &nonSeekFile{File: f}, nil
}

type nonSeekFile struct {
	fs.File
}

func TestFsShare_GetFile_OffsetNonSeekable(t *testing.T) {
	base := fstest.MapFS{
		"f.txt": &fstest.MapFile{Data: []byte("abcdef")},
	}
	s := NewFsShare(shareName, nonSeekFS{fsys: base})

	_, rc, err := s.GetFile("f.txt", 2, 0)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	defer func() {
		_ = rc.Close()
	}()

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(b) != "cdef" {
		t.Fatalf("content: got %q want %q", string(b), "cdef")
	}
}

// Optional: if you want to assert DirFiles names are base names only.
func TestFsShare_DirFiles_NamesAreBase(t *testing.T) {
	s := NewFsShare(shareName, fstest.MapFS{
		"dir/sub/file.txt": &fstest.MapFile{Data: []byte("x")},
	})

	metas, err := s.DirFiles("dir/sub")
	if err != nil {
		t.Fatalf("DirFiles error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("len: got %d want %d", len(metas), 1)
	}
	if metas[0].Name != path.Base("file.txt") {
		t.Fatalf("Name: got %q want %q", metas[0].Name, "file.txt")
	}
	if metas[0].IsDir {
		t.Fatalf("IsDir: got true want false")
	}
	if metas[0].Size != uint64(len(strings.TrimSpace("x"))) {
		// size should be 1; keep it explicit:
		t.Fatalf("Size: got %d want %d", metas[0].Size, 1)
	}
}
