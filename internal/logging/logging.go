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
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level %q: must be debug|info|warn|error", level)
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})
	return slog.New(h), nil
}
