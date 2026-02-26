package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseSubcommand(t *testing.T) {
	t.Run("no args", func(t *testing.T) {
		name, ok, err := parseSubcommand(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || name != "" {
			t.Fatalf("got name=%q ok=%v, want empty/false", name, ok)
		}
	})

	t.Run("flag arg is not subcommand", func(t *testing.T) {
		name, ok, err := parseSubcommand([]string{"-log-level", "debug"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || name != "" {
			t.Fatalf("got name=%q ok=%v, want empty/false", name, ok)
		}
	})

	t.Run("known subcommand", func(t *testing.T) {
		name, ok, err := parseSubcommand([]string{"ca-path"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok || name != "ca-path" {
			t.Fatalf("got name=%q ok=%v, want ca-path/true", name, ok)
		}
	})

	t.Run("unknown subcommand returns error", func(t *testing.T) {
		name, ok, err := parseSubcommand([]string{"nope"})
		if err == nil {
			t.Fatal("expected error for unknown subcommand")
		}
		if ok || name != "" {
			t.Fatalf("got name=%q ok=%v, want empty/false", name, ok)
		}
	})
}

func TestBoolFlagPtrIfSet(t *testing.T) {
	t.Run("unset returns nil", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		v := fs.Bool("insecure-upstream", false, "")
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := boolFlagPtrIfSet(fs, "insecure-upstream", *v); got != nil {
			t.Fatalf("got %v, want nil", *got)
		}
	})

	t.Run("set true returns pointer true", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		v := fs.Bool("insecure-upstream", false, "")
		if err := fs.Parse([]string{"-insecure-upstream=true"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := boolFlagPtrIfSet(fs, "insecure-upstream", *v)
		if got == nil || !*got {
			t.Fatalf("got %v, want pointer to true", got)
		}
	})

	t.Run("set false still returns pointer false", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		v := fs.Bool("dump-traffic", true, "")
		if err := fs.Parse([]string{"-dump-traffic=false"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := boolFlagPtrIfSet(fs, "dump-traffic", *v)
		if got == nil || *got {
			t.Fatalf("got %v, want pointer to false", got)
		}
	})
}

func TestInt64FlagPtrIfSet(t *testing.T) {
	t.Run("unset returns nil", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		v := fs.Int64("max-body-bytes", 1048576, "")
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := int64FlagPtrIfSet(fs, "max-body-bytes", *v); got != nil {
			t.Fatalf("got %v, want nil", *got)
		}
	})

	t.Run("explicit zero returns pointer zero", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		v := fs.Int64("max-body-bytes", 1048576, "")
		if err := fs.Parse([]string{"-max-body-bytes=0"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := int64FlagPtrIfSet(fs, "max-body-bytes", *v)
		if got == nil || *got != 0 {
			t.Fatalf("got %v, want pointer to 0", got)
		}
	})
}

func TestPrintStartupFailure(t *testing.T) {
	out := captureStderr(t, func() {
		printStartupFailure("failed to load config", errors.New("yaml: line 3: bad field"))
	})
	if !strings.Contains(out, "jeltz: failed to load config: yaml: line 3: bad field") {
		t.Fatalf("unexpected startup failure output: %q", out)
	}
}
