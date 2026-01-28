package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type ClientConfig struct {
	ServerAddr string  `json:"server_addr"`
	Room       string  `json:"room"`
	Username   string  `json:"username"`
	Password   string  `json:"password"`
	Shares     []Share `json:"shares,omitempty"`
	WebDAVPort int     `json:"webdav_port"`
}

type Share struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ServerAddr: "127.0.0.1:20038",
		WebDAVPort: 8080,
	}
}

func LoadClientConfig(path string) (ClientConfig, error) {
	if path == "" {
		return ClientConfig{}, errors.New("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultClientConfig(), nil
		}
		return ClientConfig{}, err
	}

	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ClientConfig{}, err
	}

	if cfg.ServerAddr == "" {
		cfg.ServerAddr = DefaultClientConfig().ServerAddr
	}
	if cfg.WebDAVPort == 0 {
		cfg.WebDAVPort = DefaultClientConfig().WebDAVPort
	}
	normalizeShares(&cfg)

	return cfg, nil
}

func SaveClientConfig(path string, cfg ClientConfig) error {
	if path == "" {
		return errors.New("config path is required")
	}

	normalizeShares(&cfg)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

func normalizeShares(cfg *ClientConfig) {
	if cfg == nil {
		return
	}
	for i := range cfg.Shares {
		if cfg.Shares[i].Name == "" && cfg.Shares[i].Path != "" {
			cfg.Shares[i].Name = filepath.Base(cfg.Shares[i].Path)
		}
	}
}
