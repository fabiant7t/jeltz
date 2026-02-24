package xdg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiant7t/jeltz/pkg/xdg"
)

func TestConfigDir_XDGEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := xdg.ConfigDir("jeltz")
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	want := filepath.Join(tmp, "jeltz")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}

func TestConfigDir_Fallback(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tmp)

	got, err := xdg.ConfigDir("jeltz")
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	want := filepath.Join(tmp, ".config", "jeltz")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDataDir_XDGEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got, err := xdg.DataDir("jeltz")
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join(tmp, "jeltz")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDataDir_Fallback(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", tmp)

	got, err := xdg.DataDir("jeltz")
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join(tmp, ".local", "share", "jeltz")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
