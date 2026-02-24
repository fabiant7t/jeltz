package logging

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestNew_AcceptsKnownLevels(t *testing.T) {
	tests := []string{"debug", "info", "warn", "error", "DEBUG", "Info"}
	for _, level := range tests {
		logger, err := New(level)
		if err != nil {
			t.Fatalf("New(%q): %v", level, err)
		}
		if logger == nil {
			t.Fatalf("New(%q): nil logger", level)
		}
	}
}

func TestNew_RejectsUnknownLevel(t *testing.T) {
	logger, err := New("verbose")
	if err == nil {
		t.Fatal("expected error for unknown level")
	}
	if logger != nil {
		t.Fatal("expected nil logger for unknown level")
	}
	if !strings.Contains(err.Error(), "must be debug|info|warn|error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_LevelFiltering(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	logger, err := New("error")
	if err != nil {
		t.Fatalf("New(error): %v", err)
	}
	logger.Info("info-should-not-appear")
	logger.Error("error-should-appear")

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "info-should-not-appear") {
		t.Fatalf("info log unexpectedly emitted: %q", got)
	}
	if !strings.Contains(got, "error-should-appear") {
		t.Fatalf("error log missing: %q", got)
	}
}
