//go:build old

package client

import (
	"context"
	"io"
	"os"
	"testing"

	pb "friendnet.org/protocol/pb/v1"
)

type fakeWebdavClient struct {
	users []string
}

func (f *fakeWebdavClient) GetOnlineUsers() ([]string, error) { return f.users, nil }
func (f *fakeWebdavClient) GetDirFiles(_ string, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeWebdavClient) GetFileMeta(_ string, _ string) (*pb.MsgFileMeta, error) {
	return nil, os.ErrNotExist
}
func (f *fakeWebdavClient) GetFile(_ string, _ string, _ uint64, _ uint64) (*pb.MsgFileMeta, io.ReadCloser, error) {
	return nil, nil, os.ErrNotExist
}

func TestSplitUserPath(t *testing.T) {
	user, sub := splitUserPath("/alice/docs/readme.txt")
	if user != "alice" || sub != "/docs/readme.txt" {
		t.Fatalf("unexpected split: %s %s", user, sub)
	}

	user, sub = splitUserPath("/bob")
	if user != "bob" || sub != "/" {
		t.Fatalf("unexpected split: %s %s", user, sub)
	}
}

func TestWebDAVRootListing(t *testing.T) {
	client := &fakeWebdavClient{users: []string{"alice", "bob"}}
	fs := &proxyFS{
		getClient: func() webdavClient { return client },
	}

	file, err := fs.OpenFile(context.Background(), "/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	infos, err := file.Readdir(-1)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(infos))
	}
	if infos[0].Name() == infos[1].Name() {
		t.Fatalf("expected distinct entries")
	}
	if !infos[0].IsDir() || !infos[1].IsDir() {
		t.Fatalf("expected directory entries")
	}
}
