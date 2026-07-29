// Package logging configures structured, journald-friendly logging for the
// daemon side (systemd, no TTY). Output is color-free and machine-readable, and
// each line is prefixed with a syslog priority (`<6>` info … `<3>` error) so
// `journalctl -p` filtering works. The interactive TUI (dashboard/wizard) keeps
// its human-facing color output and does not use this package.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Init installs the daemon slog handler as the default logger. debug forces the
// Debug level; the HEDIOUM_LOG_LEVEL env var (debug|info|warn|error) overrides.
func Init(debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	if lv, ok := parseLevel(os.Getenv("HEDIOUM_LOG_LEVEL")); ok {
		level = lv
	}
	slog.SetDefault(slog.New(NewHandler(os.Stderr, level)))
}

func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return 0, false
	}
}

// handler is a minimal slog.Handler that writes journald-friendly lines:
//
//	<pri>LEVEL message key=value key=value
type handler struct {
	level slog.Leveler
	mu    *sync.Mutex
	w     io.Writer
	attrs []slog.Attr
}

// NewHandler builds the daemon log handler writing to w at the given level.
func NewHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return &handler{level: level, mu: &sync.Mutex{}, w: w}
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(priorityPrefix(r.Level))
	b.WriteString(r.Level.String())
	b.WriteByte(' ')
	b.WriteString(r.Message)
	for _, a := range h.attrs {
		appendAttr(&b, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&b, a)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	if len(as) == 0 {
		return h
	}
	nh := *h
	nh.attrs = append(append([]slog.Attr(nil), h.attrs...), as...)
	return &nh
}

// WithGroup is a no-op: this daemon does not use attribute groups.
func (h *handler) WithGroup(string) slog.Handler { return h }

func appendAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	b.WriteByte(' ')
	b.WriteString(a.Key)
	b.WriteByte('=')
	b.WriteString(a.Value.String())
}

// priorityPrefix maps a slog level to an sd-daemon syslog priority prefix.
func priorityPrefix(l slog.Level) string {
	var p int
	switch {
	case l >= slog.LevelError:
		p = 3 // err
	case l >= slog.LevelWarn:
		p = 4 // warning
	case l >= slog.LevelInfo:
		p = 6 // info
	default:
		p = 7 // debug
	}
	return "<" + strconv.Itoa(p) + ">"
}
