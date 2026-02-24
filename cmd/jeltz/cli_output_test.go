package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(out)
}

func TestRunCAPath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	out := captureStdout(t, runCAPath)
	got := strings.TrimSpace(out)
	want := filepath.Join(base, "jeltz", "ca.crt.pem")
	if got != want {
		t.Fatalf("ca path: got %q, want %q", got, want)
	}
}

func TestRunCAP12Path(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	out := captureStdout(t, runCAP12Path)
	got := strings.TrimSpace(out)
	want := filepath.Join(base, "jeltz", "ca.p12")
	if got != want {
		t.Fatalf("p12 path: got %q, want %q", got, want)
	}
}

func TestRunCAInstallHint(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	out := captureStdout(t, runCAInstallHint)
	certPath := filepath.Join(base, "jeltz", "ca.crt.pem")
	p12Path := filepath.Join(base, "jeltz", "ca.p12")

	if !strings.Contains(out, "jeltz CA Certificate Installation Hints") {
		t.Fatalf("missing heading: %q", out)
	}
	if !strings.Contains(out, certPath) {
		t.Fatalf("missing cert path %q in output", certPath)
	}
	if !strings.Contains(out, p12Path) {
		t.Fatalf("missing p12 path %q in output", p12Path)
	}
}

func TestPrintBanner_BasicContent_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	oldVersion, oldBuildDate, oldRevision := version, buildDate, gitRevision
	version = "test-version"
	buildDate = "2026-02-24"
	gitRevision = "deadbeef"
	t.Cleanup(func() {
		version = oldVersion
		buildDate = oldBuildDate
		gitRevision = oldRevision
	})

	out := captureStderr(t, func() {
		printBanner(
			"127.0.0.1:8080",
			"/tmp/config.yaml",
			"/tmp/data",
			"/tmp/data/ca.crt.pem",
			"/tmp/data/ca.p12",
			3,
			"debug",
			false,
			true,
		)
	})

	if strings.Contains(out, "\x1b[") {
		t.Fatalf("unexpected ANSI escape codes in NO_COLOR mode: %q", out)
	}
	if !strings.Contains(out, "jeltz") {
		t.Fatalf("missing banner title")
	}
	if !strings.Contains(out, "DON'T PANIC") {
		t.Fatalf("missing panic line")
	}
	if !strings.Contains(out, "/tmp/config.yaml") {
		t.Fatalf("missing config path")
	}
	if !strings.Contains(out, "dump") || !strings.Contains(out, "on") {
		t.Fatalf("missing dump state")
	}
}
