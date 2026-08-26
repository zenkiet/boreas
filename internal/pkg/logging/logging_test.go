package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestLevelNames(t *testing.T) {
	for name, want := range map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"info":    slog.LevelInfo,
		"":        slog.LevelInfo,
		"bogus":   slog.LevelInfo,
	} {
		if got := Level(name); got != want {
			t.Errorf("Level(%q) = %v, want %v", name, got, want)
		}
	}
	if got := Level(" warn "); got != slog.LevelWarn {
		t.Errorf("Level with surrounding spaces = %v, want %v", got, slog.LevelWarn)
	}
}

func TestNewEmitsJSONWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	logger.Info("task started", "project", "demo", "task", "web")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v: %q", err, buf.String())
	}
	if entry["msg"] != "task started" || entry["project"] != "demo" || entry["task"] != "web" {
		t.Fatalf("unexpected entry: %v", entry)
	}
	if entry["level"] != "INFO" {
		t.Fatalf("level = %v, want INFO", entry["level"])
	}
}

func TestNewFiltersBelowConfiguredLevel(t *testing.T) {
	t.Setenv("BOREAS_LOG_LEVEL", "error")
	var buf bytes.Buffer
	logger := New(&buf)

	logger.Info("dropped")
	if buf.Len() != 0 {
		t.Fatalf("info line should be filtered at error level: %q", buf.String())
	}
	logger.Error("kept")
	if buf.Len() == 0 {
		t.Fatal("error line should pass at error level")
	}
}
