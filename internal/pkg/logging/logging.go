// Package logging builds the process-wide structured logger.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON slog.Logger writing to w, filtering below the level
// named by BOREAS_LOG_LEVEL. JSON keeps log lines machine-parseable for
// collectors without a format flag to configure.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: Level(os.Getenv("BOREAS_LOG_LEVEL")),
	}))
}

// Level maps a level name to its slog level. Unknown or empty names mean
// info, so a typo raises verbosity rather than silencing errors.
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
