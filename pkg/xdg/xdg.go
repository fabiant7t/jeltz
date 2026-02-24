// Package xdg resolves XDG Base Directory Specification paths.
package xdg

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the application config directory, creating it if absent.
// Uses $XDG_CONFIG_HOME or $HOME/.config as the base.
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

// DataDir returns the application data directory, creating it if absent.
// Uses $XDG_DATA_HOME or $HOME/.local/share as the base.
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
