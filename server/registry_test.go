package main

import (
	"testing"

	"friendnet.org/protocol"
)

func TestClientRegistry(t *testing.T) {
	registry := NewClientRegistry()
	client := &protocol.ProtoServerClient{}

	if err := registry.Register(client, "Room", "Alice"); err != nil {
		t.Fatalf("register: %v", err)
	}

	info, ok := registry.Info(client)
	if !ok || info.Room != "Room" || info.Username != "Alice" {
		t.Fatalf("unexpected info: %+v", info)
	}

	lookup := registry.Lookup("room", "alice")
	if lookup != client {
		t.Fatalf("expected lookup to return client")
	}

	registry.Unregister(client)
	if registry.Lookup("room", "alice") != nil {
		t.Fatalf("expected client to be unregistered")
	}
}
