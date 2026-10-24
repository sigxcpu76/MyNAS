package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type stater interface {
	Stat() (os.FileInfo, error)
}

// IsTTY detects whether the given writer is attached to an interactive terminal (character device).
func IsTTY(w io.Writer) bool {
	if s, ok := w.(stater); ok {
		fi, err := s.Stat()
		if err == nil {
			return (fi.Mode() & os.ModeCharDevice) != 0
		}
	}
	return false
}

// ParseLevel converts a string level ("debug", "info", "warn", "error") into slog.Level.
func ParseLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
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

// NewLogger creates a standard library slog.Logger that automatically outputs
// human-readable non-JSON text logs when running on a TTY, and structured JSON
// logs when not running on a TTY (e.g., pipes, systemd, containers).
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	opts := &slog.HandlerOptions{
		Level: level,
	}
	var handler slog.Handler
	if IsTTY(w) {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}
