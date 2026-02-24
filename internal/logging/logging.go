// Package logging provides slog setup and stable key constants for jeltz.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Stable slog attribute keys.
const (
	KeyComponent  = "component"
	KeyEvent      = "event"
	KeyClient     = "client"
	KeyMethod     = "method"
	KeyScheme     = "scheme"
	KeyHost       = "host"
	KeyPath       = "path"
	KeyStatus     = "status"
	KeySource     = "source"
	KeyDurationMS = "duration_ms"
	KeyProto      = "proto"
	KeyError      = "error"
)

// New builds a slog.Logger with TextHandler at the given level string.
// level must be one of: debug, info, warn, error (case-insensitive).
func New(level string) (*slog.Logger, error) {
	l, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})
	return slog.New(h), nil
}

// ParseLevel parses a log level string into slog.Level.
func ParseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q: must be debug|info|warn|error", level)
	}
}
