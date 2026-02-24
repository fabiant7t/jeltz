package logstream

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Event is a UI-consumable log record.
type Event struct {
	Time      time.Time
	Level     slog.Level
	Message   string
	Component string
}

// Stream is a bounded non-blocking log event queue.
type Stream struct {
	ch      chan Event
	dropped atomic.Uint64
}

// New creates a stream with the given buffer size.
func New(size int) *Stream {
	if size < 1 {
		size = 1
	}
	return &Stream{ch: make(chan Event, size)}
}

// Events returns the stream channel.
func (s *Stream) Events() <-chan Event { return s.ch }

// Dropped returns the number of log events dropped due to queue pressure.
func (s *Stream) Dropped() uint64 { return s.dropped.Load() }

// Handler returns a slog.Handler writing to this stream.
func (s *Stream) Handler(minLevel slog.Level) slog.Handler {
	return &handler{
		stream:   s,
		minLevel: minLevel,
	}
}

type handler struct {
	stream    *Stream
	minLevel  slog.Level
	attrs     []slog.Attr
	groupPath []string
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	ev := Event{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
	}

	for _, a := range h.attrs {
		if a.Key == "component" {
			if v, ok := a.Value.Any().(string); ok {
				ev.Component = v
			}
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			if v, ok := a.Value.Any().(string); ok {
				ev.Component = v
			}
		}
		return true
	})

	select {
	case h.stream.ch <- ev:
	default:
		h.stream.dropped.Add(1)
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *handler) WithGroup(name string) slog.Handler {
	next := *h
	next.groupPath = append(append([]string{}, h.groupPath...), name)
	return &next
}
