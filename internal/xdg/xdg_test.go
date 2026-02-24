package xdg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabiant7t/jeltz/internal/xdg"
)

func TestConfigDir_XDGEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := xdg.ConfigDir()
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

	got, err := xdg.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "jeltz")) {
		t.Errorf("unexpected path: %q", got)
	}
}

func TestDataDir_XDGEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got, err := xdg.DataDir()
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

	got, err := xdg.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".local", "share", "jeltz")) {
		t.Errorf("unexpected path: %q", got)
	}
}
