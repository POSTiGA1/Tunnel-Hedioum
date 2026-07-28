package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestHandlerFormatAndPriority(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(&buf, slog.LevelDebug))

	log.Info("hello", "node", "de-01", "count", 3)
	out := buf.String()
	if !strings.HasPrefix(out, "<6>INFO hello") {
		t.Fatalf("info line: %q", out)
	}
	if !strings.Contains(out, "node=de-01") || !strings.Contains(out, "count=3") {
		t.Fatalf("attrs missing: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("output must be color-free: %q", out)
	}

	buf.Reset()
	log.Warn("careful")
	if !strings.HasPrefix(buf.String(), "<4>WARN") {
		t.Fatalf("warn priority: %q", buf.String())
	}

	buf.Reset()
	log.Error("boom")
	if !strings.HasPrefix(buf.String(), "<3>ERROR") {
		t.Fatalf("error priority: %q", buf.String())
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, slog.LevelInfo)
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("debug should be filtered at info level")
	}
	log := slog.New(h)
	log.Debug("should-not-appear")
	if buf.Len() != 0 {
		t.Fatalf("debug leaked: %q", buf.String())
	}
	log.Info("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("info should appear: %q", buf.String())
	}
}

func TestWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(&buf, slog.LevelInfo)).With("role", "egress")
	log.Info("up")
	if !strings.Contains(buf.String(), "role=egress") {
		t.Fatalf("With attr missing: %q", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "INFO": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn, "error": slog.LevelError,
	} {
		if lv, ok := parseLevel(in); !ok || lv != want {
			t.Errorf("parseLevel(%q) = %v,%v want %v", in, lv, ok, want)
		}
	}
	if _, ok := parseLevel("bogus"); ok {
		t.Error("bogus level should not parse")
	}
}
