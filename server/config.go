package main

import (
	"encoding/json"
	"errors"
	"os"
)

type ServerConfig struct {
	Listen []string     `json:"listen"`
	Rooms  []RoomConfig `json:"rooms"`
}

type RoomConfig struct {
	Name  string       `json:"name"`
	Users []UserConfig `json:"users"`
}

type UserConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Listen: []string{
			"127.0.0.1:20038",
			"[::1]:20038",
		},
	}
}

func LoadServerConfig(path string) (ServerConfig, error) {
	if path == "" {
		return ServerConfig{}, errors.New("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist, write default config.
			def := DefaultServerConfig()
			data, err = json.MarshalIndent(def, "", "  ")
			if err != nil {
				return ServerConfig{}, err
			}
			err = os.WriteFile(path, data, 0o600)
			return def, err
		}
		return ServerConfig{}, err
	}

	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ServerConfig{}, err
	}

	if len(cfg.Listen) == 0 {
		cfg.Listen = DefaultServerConfig().Listen
	}

	return cfg, nil
}
