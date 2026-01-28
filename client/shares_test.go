package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShareResolvePath(t *testing.T) {
	dir := t.TempDir()
	shareDir := filepath.Join(dir, "music")
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := ClientConfig{
		Shares: []Share{
			{Name: "audio", Path: shareDir},
		},
	}

	manager := NewShareManager(&cfg)
	resolved, err := manager.ResolvePath("/audio/song.mp3")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if filepath.Dir(resolved) != shareDir {
		t.Fatalf("unexpected resolved path: %s", resolved)
	}
}

func TestShareListDir(t *testing.T) {
	dir := t.TempDir()
	shareDir := filepath.Join(dir, "share")
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shareDir, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := ClientConfig{
		Shares: []Share{
			{Name: "share", Path: shareDir},
		},
	}
	manager := NewShareManager(&cfg)

	resp, err := manager.ListDir("/share")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Filenames) != 1 || resp.Filenames[0] != "file.txt" {
		t.Fatalf("unexpected filenames: %v", resp.Filenames)
	}
}

func TestShareListRoot(t *testing.T) {
	dir := t.TempDir()
	shareDir := filepath.Join(dir, "share")
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := ClientConfig{
		Shares: []Share{
			{Name: "docs", Path: shareDir},
		},
	}
	manager := NewShareManager(&cfg)

	resp, err := manager.ListDir("/")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Filenames) != 1 || resp.Filenames[0] != "docs" {
		t.Fatalf("unexpected filenames: %v", resp.Filenames)
	}
}
