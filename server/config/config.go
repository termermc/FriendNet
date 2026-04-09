package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"friendnet.org/common"
)

// DefaultRpcPemPath is the default path to the RPC HTTPS certificate file.
const DefaultRpcPemPath = "rpc.pem"

// ServerRpcConfig is the configuration for the server's RPC service.
type ServerRpcConfig struct {
	// HttpsPemPath is the path to the full chain certificate to use for serving RPC endpoints over HTTPS.
	HttpsPemPath string `json:"https_pem_path"`

	// Interfaces is a list of RPC server interfaces and their settings.
	Interfaces []common.RpcServerConfig `json:"interfaces"`
}

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

	// If true, the server will periodically check for updates and log to the console if a new version is available.
	DisableUpdateChecker bool `json:"disable_update_checker"`

	// The configuration for the server's RPC service.
	Rpc ServerRpcConfig `json:"rpc"`
}

// Default is the default server configuration.
var Default = &ServerConfig{
	Listen: []string{
		"0.0.0.0:20038",
		"[::]:20038",
	},
	DbPath:               "server.db",
	PemPath:              "server.pem",
	DisableUpdateChecker: false,

	Rpc: ServerRpcConfig{
		HttpsPemPath: DefaultRpcPemPath,
		Interfaces: []common.RpcServerConfig{
			{
				Address:        "unix://friendnet-server.sock",
				AllowedMethods: []string{"*"},
			},
			{
				Address: "http://127.0.0.1:8080",
				AllowedMethods: []string{
					"GetRooms",
					"GetRoomInfo",
					"GetOnlineUsers",
					"GetOnlineUserInfo",
				},
				CorsAllowAllOrigins: true,
			},
		},
	},
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

	// Ensure all RPC interface addresses are valid URLs.
	for _, iface := range cfg.Rpc.Interfaces {
		_, err = url.Parse(iface.Address)
		if err != nil {
			return nil, fmt.Errorf(`interface address %q is not a valid URL: %w`, iface.Address, err)
		}
	}

	return &cfg, nil
}
