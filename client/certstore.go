package main

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
)

type JSONCertStore struct {
	path  string
	state *ClientState
	mu    sync.Mutex
}

func NewJSONCertStore(path string, state *ClientState) (*JSONCertStore, error) {
	if path == "" {
		return nil, errors.New("state path is required")
	}
	if state == nil {
		return nil, errors.New("state is required")
	}
	if state.Certs == nil {
		state.Certs = make(map[string]string)
	}

	return &JSONCertStore{
		path:  path,
		state: state,
	}, nil
}

func (s *JSONCertStore) Get(host string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if host == "" {
		return nil, nil
	}
	host = strings.ToLower(host)

	encoded := s.state.Certs[host]
	if encoded == "" {
		return nil, nil
	}

	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	return der, nil
}

func (s *JSONCertStore) Put(host string, certDER []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if host == "" {
		return errors.New("host is required")
	}
	host = strings.ToLower(host)

	s.state.Certs[host] = base64.StdEncoding.EncodeToString(certDER)
	return SaveClientState(s.path, *s.state)
}
