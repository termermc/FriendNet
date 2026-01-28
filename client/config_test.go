package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.json")

	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("load default: %v", err)
	}
	if cfg.ServerAddr == "" {
		t.Fatalf("expected default server addr")
	}

	cfg.Room = "room1"
	cfg.Username = "user1"
	cfg.Password = "pass1"
	if err := SaveClientConfig(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Room != "room1" || loaded.Username != "user1" || loaded.Password != "pass1" {
		t.Fatalf("unexpected config: %+v", loaded)
	}
}

func TestStateLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	state, err := LoadClientState(path)
	if err != nil {
		t.Fatalf("load default: %v", err)
	}
	if state.Certs == nil {
		t.Fatalf("expected cert map")
	}

	state.LastAddr = "127.0.0.1:20038"
	state.Certs["example.com"] = "abc"
	if err := SaveClientState(path, state); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadClientState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.LastAddr != state.LastAddr || loaded.Certs["example.com"] != "abc" {
		t.Fatalf("unexpected state: %+v", loaded)
	}
}

func TestJSONCertStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	state := DefaultClientState()
	if err := SaveClientState(path, state); err != nil {
		t.Fatalf("save initial: %v", err)
	}

	store, err := NewJSONCertStore(path, &state)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.Put("Example.com", []byte{0x01, 0x02}); err != nil {
		t.Fatalf("put: %v", err)
	}

	der, err := store.Get("example.com")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(der) != 2 || der[0] != 0x01 || der[1] != 0x02 {
		t.Fatalf("unexpected der: %v", der)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file: %v", err)
	}
}
