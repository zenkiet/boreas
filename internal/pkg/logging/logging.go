// Package logging builds the process-wide structured logger.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: Level(os.Getenv("BOREAS_LOG_LEVEL")),
	}))
}

func Level(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
