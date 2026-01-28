package main

import (
	"fmt"
	"strings"
	"sync"

	"friendnet.org/protocol"
)

type ClientInfo struct {
	Room     string
	Username string
}

type ClientRegistry struct {
	mu     sync.RWMutex
	byUser map[string]map[string]*protocol.ProtoServerClient
	byConn map[*protocol.ProtoServerClient]ClientInfo
}

func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{
		byUser: make(map[string]map[string]*protocol.ProtoServerClient),
		byConn: make(map[*protocol.ProtoServerClient]ClientInfo),
	}
}

func (r *ClientRegistry) Register(client *protocol.ProtoServerClient, room string, username string) error {
	if client == nil {
		return fmt.Errorf("client is required")
	}
	roomKey := strings.ToLower(room)
	userKey := strings.ToLower(username)
	if roomKey == "" || userKey == "" {
		return fmt.Errorf("room and username are required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	roomMap := r.byUser[roomKey]
	if roomMap == nil {
		roomMap = make(map[string]*protocol.ProtoServerClient)
		r.byUser[roomKey] = roomMap
	}
	roomMap[userKey] = client
	r.byConn[client] = ClientInfo{
		Room:     room,
		Username: username,
	}

	return nil
}

func (r *ClientRegistry) Unregister(client *protocol.ProtoServerClient) {
	if client == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.byConn[client]
	if !ok {
		return
	}
	delete(r.byConn, client)

	roomKey := strings.ToLower(info.Room)
	userKey := strings.ToLower(info.Username)
	if roomMap, ok := r.byUser[roomKey]; ok {
		delete(roomMap, userKey)
		if len(roomMap) == 0 {
			delete(r.byUser, roomKey)
		}
	}
}

func (r *ClientRegistry) Lookup(room string, username string) *protocol.ProtoServerClient {
	roomKey := strings.ToLower(room)
	userKey := strings.ToLower(username)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if roomMap, ok := r.byUser[roomKey]; ok {
		return roomMap[userKey]
	}
	return nil
}

func (r *ClientRegistry) Info(client *protocol.ProtoServerClient) (ClientInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.byConn[client]
	return info, ok
}

func (r *ClientRegistry) ListUsers(room string) []string {
	roomKey := strings.ToLower(room)

	r.mu.RLock()
	defer r.mu.RUnlock()

	roomMap, ok := r.byUser[roomKey]
	if !ok {
		return nil
	}

	users := make([]string, 0, len(roomMap))
	for _, client := range roomMap {
		if info, ok := r.byConn[client]; ok {
			users = append(users, info.Username)
		}
	}
	return users
}
