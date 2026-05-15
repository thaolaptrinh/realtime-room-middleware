package observability

import (
	"io"
	"log/slog"
	"os"
)

type Level = slog.Level

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

func parseLevel(s string) (Level, bool) {
	switch s {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn":
		return LevelWarn, true
	case "error":
		return LevelError, true
	default:
		return LevelInfo, false
	}
}

// InitLogger initializes a structured JSON logger writing to w.
// levelStr must be one of "debug", "info", "warn", "error".
// Returns the default logger and the parsed level.
func InitLogger(w io.Writer, levelStr string) (*slog.Logger, Level) {
	level, ok := parseLevel(levelStr)
	if !ok {
		level = LevelInfo
	}

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, level
}

// InitDefaultLogger initializes the logger with stdout and the given level string.
func InitDefaultLogger(levelStr string) *slog.Logger {
	logger, _ := InitLogger(os.Stdout, levelStr)
	return logger
}
