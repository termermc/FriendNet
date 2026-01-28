package main

import (
	"encoding/json"
	"errors"
	"os"
)

type ClientState struct {
	Certs        map[string]string `json:"certs"`
	LastAddr     string            `json:"last_addr,omitempty"`
	LastRoom     string            `json:"last_room,omitempty"`
	LastUsername string            `json:"last_username,omitempty"`
	LastSeenAt   string            `json:"last_seen_at,omitempty"`
}

func DefaultClientState() ClientState {
	return ClientState{
		Certs: make(map[string]string),
	}
}

func LoadClientState(path string) (ClientState, error) {
	if path == "" {
		return ClientState{}, errors.New("state path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultClientState(), nil
		}
		return ClientState{}, err
	}

	var state ClientState
	if err := json.Unmarshal(data, &state); err != nil {
		return ClientState{}, err
	}
	if state.Certs == nil {
		state.Certs = make(map[string]string)
	}

	return state, nil
}

func SaveClientState(path string, state ClientState) error {
	if path == "" {
		return errors.New("state path is required")
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}
