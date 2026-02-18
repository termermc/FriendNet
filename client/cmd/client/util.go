package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetDataDir returns an appropriate per-user, per-app data directory.
//
// Conventions:
//   - Linux/Unix: $XDG_DATA_HOME/<appName> or ~/.local/share/<appName>
//   - macOS: ~/Library/Application Support/<appName>
//   - Windows: %AppData%\<appName> (roaming) with fallbacks
//
// It does not create the directory; call os.MkdirAll on the result.
func GetDataDir() (string, error) {
	const appName = "friendnet-client"

	switch runtime.GOOS {
	case "windows":
		// Prefer Roaming AppData.
		if base := os.Getenv("APPDATA"); base != "" {
			return filepath.Join(base, appName), nil
		}
		// Fallback to LocalAppData.
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, appName), nil
		}
		// Last resort: user home.
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "AppData", "Roaming", appName), nil

	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", appName), nil

	default: // linux, freebsd, openbsd, netbsd, etc.
		// Follow XDG Base Directory spec where applicable.
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, appName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", appName), nil
	}
}
