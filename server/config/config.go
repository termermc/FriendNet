package config

import (
	"encoding/json"
	"errors"
	"os"
)

// ServerRpcConfigInterface is an interface within ServerRpcConfig.
type ServerRpcConfigInterface struct {
	// The address to bind to.
	// Must be in the format "PROTOCOL://HOST:PORT" (or without port for unix).
	//
	// Supported protocols:
	//  - http
	//  - unix
	//
	// Examples:
	//  - "http://127.0.0.1:8080"
	//  - "unix:///var/run/friendnet-server.sock" (/var/run/friendnet-server.sock, absolute path)
	//  - "unix://friendnet-server.sock" (friendnet-server.sock, relative path)
	//
	// The unix protocol will create a file with 0600 permission by default.
	// To set the permission, set the "file_permission" field.
	// Windows support for the unix protocol is not supported but may work.
	Address string `json:"address"`

	// The RPC methods that are allowed to be called on this interface.
	// Consult the rpc.proto file to see a full list of methods.
	//
	// An empty or null list will prevent any methods from being called.
	//
	// To explicitly allow all methods, include a single string with the value "*".
	//
	// Example: ["GetRooms", "GetRoomInfo", "GetOnlineUsers", "GetOnlineUserInfo"]
	AllowedMethods []string `json:"allowed_methods"`

	// If not null or empty, only the specified IP addresses will be allowed to connect.
	// Has no effect if the address protocol is unix.
	AllowedIps []string `json:"allowed_ips,omitempty"`

	// The file mode to use for the unix socket file.
	// Must be in format "0600", starting with a zero followed by three octal digits.
	// Has no effect if the address protocol is not unix.
	FilePermission string `json:"file_permission,omitempty"`

	// If not null or empty, the following HTTP bearer token will be required to access the RPC interface.
	// For example, if set to "abc123", the following HTTP header must be set: "Authorization: Bearer abc123".
	BearerToken string `json:"bearer_token,omitempty"`
}

// ServerRpcConfig is the configuration for the server's RPC service.
type ServerRpcConfig struct {
	// Interfaces is a list of RPC server interfaces and their settings.
	Interfaces []ServerRpcConfigInterface `json:"listeners"`
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

	// The configuration for the server's RPC service.
	Rpc ServerRpcConfig `json:"rpc"`
}

// Default is the default server configuration.
var Default = &ServerConfig{
	Listen: []string{
		"127.0.0.1:20038",
		"[::1]:20038",
	},
	DbPath:  "server.db",
	PemPath: "server.pem",

	Rpc: ServerRpcConfig{
		Interfaces: []ServerRpcConfigInterface{
			{
				Address:        "unix://friendnet-server.sock",
				AllowedMethods: []string{"*"},
			},
			{
				Address: "http://0.0.0.0:8080",
				AllowedMethods: []string{
					"GetRooms",
					"GetRoomInfo",
					"GetOnlineUsers",
					"GetOnlineUserInfo",
				},
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

	return &cfg, nil
}
