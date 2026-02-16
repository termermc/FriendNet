package server

import (
	"context"
	"sync"

	"friendnet.org/server/config"
)

// RpcServer implements an RPC server for a FriendNet server.
// It is a single instance that runs on a single interface.
type RpcServer struct {
	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	iface  config.ServerRpcConfigInterface
	server *Server

	isAllAllowed   bool
	allowedMethods map[string]struct{}
}

func NewRpcServer(iface config.ServerRpcConfigInterface, server *Server) *RpcServer {
	ctx, cancel := context.WithCancel(context.Background())

	var isAllAllowed bool
	var allowedMethods map[string]struct{}
	if len(iface.AllowedMethods) == 1 && iface.AllowedMethods[0] == "*" {
		isAllAllowed = true
		allowedMethods = nil
	} else {
		isAllAllowed = false
		allowedMethods = make(map[string]struct{}, len(iface.AllowedMethods))
		for _, method := range iface.AllowedMethods {
			allowedMethods[method] = struct{}{}
		}
	}

	return &RpcServer{
		ctx:       ctx,
		ctxCancel: cancel,

		iface:  iface,
		server: server,

		isAllAllowed:   isAllAllowed,
		allowedMethods: make(map[string]struct{}),
	}
}

func (s *RpcServer) isMethodAllowed(method string) bool {
	if s.isAllAllowed {
		return true
	}

	_, ok := s.allowedMethods[method]
	return ok
}

func (s *RpcServer) Close() error {
	s.mu.Lock()
	if s.isClosed {
		s.mu.Unlock()
		return nil
	}
	s.isClosed = true
	s.mu.Unlock()

	// TODO Close server (including delete socket if UNIX protocol)
	return nil
}
