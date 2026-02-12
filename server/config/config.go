package config

import (
	"encoding/json"
	"errors"
	"os"
)

// ServerConfig is the server configuration.
type ServerConfig struct {
	// The addresses to listen on.
	// Each entry should be HOST:PORT.
	// IPv6 addresses should be enclosed in square brackets (like "[::1]:20038").
	Listen []string `json:"listen"`

	// The path (relative or absolute) to the SQLite database file.
	// Will be created if it does not exist.
	DbPath string `json:"db_path"`

	// The path (relative or absolute) to the TLS certificate file in PEM format.
	// A new self-signed certificate will be generated if it does not exist.
	PemPath string `json:"pem_path"`
}

// Default is the default server configuration.
var Default = &ServerConfig{
	Listen: []string{
		"127.0.0.1:20038",
		"[::1]:20038",
	},
	DbPath:  "server.db",
	PemPath: "server.pem",
}

// LoadOrCreate loads the server configuration at the specified path.
// If the file does not exist, it will be created using values from Default.
// Returns an error if the file is invalid.
func LoadOrCreate(path string) (*ServerConfig, error) {
	if path == "" {
		return nil, errors.New("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist, write default config.
			data, err = json.MarshalIndent(Default, "", "  ")
			if err != nil {
				return nil, err
			}
			err = os.WriteFile(path, data, 0o600)
			return Default, err
		}
		return nil, err
	}

	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.DbPath == "" {
		return nil, errors.New("db_path is required")
	}
	if cfg.PemPath == "" {
		return nil, errors.New("pem_path is required")
	}
	if len(cfg.Listen) == 0 {
		return nil, errors.New("at least one listen address is required")
	}

	return &cfg, nil
}
