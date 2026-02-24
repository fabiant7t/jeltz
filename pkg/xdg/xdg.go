// Package xdg resolves XDG Base Directory paths for jeltz.
package xdg

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the jeltz config directory, creating it if absent.
// Uses $XDG_CONFIG_HOME/jeltz or $HOME/.config/jeltz.
func ConfigDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "jeltz")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// DataDir returns the jeltz data directory, creating it if absent.
// Uses $XDG_DATA_HOME/jeltz or $HOME/.local/share/jeltz.
func DataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "jeltz")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
