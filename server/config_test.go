package main

import (
	"context"
	"path/filepath"
	"testing"

	pb "friendnet.org/protocol/pb/v1"
)

func TestServerConfigLoadDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")

	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("load default: %v", err)
	}
	if len(cfg.Listen) == 0 {
		t.Fatalf("expected default listen addresses")
	}
}

func TestAuthStore(t *testing.T) {
	cfg := ServerConfig{
		Rooms: []RoomConfig{
			{
				Name: "room1",
				Users: []UserConfig{
					{Username: "alice", Password: "secret"},
				},
			},
		},
	}

	store, err := NewAuthStore(cfg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	accepted, rejected, err := store.Handler(context.Background(), nil, &pb.MsgAuthenticate{
		Room:     "room1",
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if accepted == nil || rejected != nil {
		t.Fatalf("expected acceptance")
	}

	accepted, rejected, err = store.Handler(context.Background(), nil, &pb.MsgAuthenticate{
		Room:     "room1",
		Username: "alice",
		Password: "bad",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if accepted != nil || rejected == nil {
		t.Fatalf("expected rejection")
	}
}
