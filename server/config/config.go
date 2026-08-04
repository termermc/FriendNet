package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

// ServerHttpAuthConfig is configuration for an HTTP external authentication provider.
type ServerHttpAuthConfig struct {
	// The endpoint's URL.
	// Supports HTTP, HTTPS and UNIX socket.
	//
	// If UNIX socket, the URL must be formatted like `unix:///path/to/auth.sock` (note the triple slashes), or just a
	// double slash for a path relative to the server's current working directory, such as `unix://auth.sock`.
	Url string `json:"url"`
}

// ServerCommandAuthConfig is configuration for a command or script external authentication provider.
type ServerCommandAuthConfig struct {
	// The script's name or path.
	// For example: "/usr/local/bin/auth.sh".
	//
	// If not an absolute ("/") or relative ("./") path, it will look for a program with this name in the system's PATH.
	// Relative paths are relative to the server's current working directory.
	//
	// Note that on UNIX-like operating systems (such as Linux or Mac), the script must be marked as executable.
	Name string `json:"name"`

	// Any arguments to pass to the script.
	// Can be empty or omitted.
	Args []string `json:"args,omitempty"`
}

// ServerAuthProviderConfig is the configuration for an external authentication provider.
// At least "http" or "command" must be specified, but both cannot be specified at once.
type ServerAuthProviderConfig struct {
	// The timeout in seconds to wait on a response from the external authentication provider before giving up.
	// If omitted or zero, defaults to 10 seconds.
	TimeoutSeconds uint `json:"timeout_seconds,omitempty"`

	// If set, uses an HTTP endpoint as the authentication provider.
	Http *ServerHttpAuthConfig `json:"http,omitempty"`

	// If set, uses a command or script as the authentication provider.
	Command *ServerCommandAuthConfig `json:"command,omitempty"`
}

// ServerRoomExternalAuthConfig is external authentication configuration for a specific room.
type ServerRoomExternalAuthConfig struct {
	// Authentication providers to use for the room.
	Providers []ServerAuthProviderConfig `json:"providers"`

	// If true, the room's providers will be used before global providers.
	BeforeGlobal bool `json:"before_global"`
}

// ServerExternalAuthConfig is external authentication configuration.
// See https://friendnet.org/docs/server/external-authentication
type ServerExternalAuthConfig struct {
	// Global authentication providers.
	// Applies to all rooms.
	Global []ServerAuthProviderConfig

	// A mapping of room names to specific authentication providers for them.
	Rooms map[string]ServerRoomExternalAuthConfig
}

// ServerConfig is the server configuration.
type ServerConfig struct {
	// JsonSchema is a placeholder field to hold the JSON schema URL for validation.
	JsonSchema string `json:"$schema,omitempty"`

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

	// If true, the server will NOT periodically check for updates and log to the console if a new version is available.
	DisableUpdateChecker bool `json:"disable_update_checker"`

	// List of STUN servers to return to clients.
	// In most cases, this should just contain the public address and port of your FriendNet server, because the server
	// exposes a STUN server on the same port.
	// Each entry should be HOST:PORT.
	// IPv6 addresses should be enclosed in square brackets (like "[::1]:20038").
	// If empty, the server will try to guess the address of its built-in STUN server.
	// Examples:
	//  - "my.friendnet.server:20038"
	//  - "stun.l.google.com:19302"
	StunServers []string `json:"stun_servers,omitempty"`

	// The configuration for the server's RPC service.
	Rpc ServerRpcConfig `json:"rpc"`

	// External authentication configuration.
	// See https://friendnet.org/docs/server/external-authentication
	ExternalAuth *ServerExternalAuthConfig `json:"external_auth,omitempty"`
}

// Default is the default server configuration.
var Default = &ServerConfig{
	JsonSchema: common.ServerCfgJsonSchemaUrl,

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

func validateAuthProvider(path string, p *ServerAuthProviderConfig) error {
	if p.Http == nil && p.Command == nil {
		return fmt.Errorf(`%s: external auth provider configs must specify at least "http" or "command"`, path)
	}
	if p.Http != nil && p.Command != nil {
		return fmt.Errorf(`%s: external auth provider configs cannot specify both "http" and "command"`, path)
	}

	if p.TimeoutSeconds < 1 {
		p.TimeoutSeconds = 10
	}

	if p.Http != nil {
		if p.Http.Url == "" {
			return fmt.Errorf(`%s.http: missing HTTP auth provider URL`, path)
		}

		u, err := url.Parse(p.Http.Url)
		if err != nil {
			return fmt.Errorf(`%s.http.url: HTTP auth provider URL is invalid: %w`, path, err)
		}

		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "unix" {
			return fmt.Errorf(`%s.http.url: invalid HTTP auth provider scheme %q, must be "http", "https" or "unix"`, path, u.Scheme)
		}
	} else if p.Command != nil {
		if p.Command.Name == "" {
			return fmt.Errorf(`%s.command: missing command auth provider command/script name`, path)
		}
	}

	return nil
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

	// Ensure all STUN server addresses are valid.
	for _, server := range cfg.StunServers {
		_, _, err = net.SplitHostPort(server)
		if err != nil {
			return nil, fmt.Errorf(`STUN server address %q is not a valid HOST:PORT address: %w`, server, err)
		}
	}

	// Validate external auth providers.
	if cfg.ExternalAuth != nil {
		ext := cfg.ExternalAuth

		for _, prov := range ext.Global {
			if err = validateAuthProvider("external_auth.global", &prov); err != nil {
				return nil, err
			}
		}

		for roomName, roomCfg := range ext.Rooms {
			if _, ok := common.NormalizeRoomName(roomName); !ok {
				return nil, fmt.Errorf(`external_auth.rooms: %q is not a valid room name`, roomName)
			}

			for _, prov := range roomCfg.Providers {
				if err = validateAuthProvider("external_auth.rooms."+roomName, &prov); err != nil {
					return nil, err
				}
			}
		}
	}

	return &cfg, nil
}
