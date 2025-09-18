package logging

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	loggerMu sync.RWMutex
	logger   *slog.Logger
)

func init() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// Logger returns the process-wide structured logger.
func Logger() *slog.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return logger
}

// SetLogger overrides the global logger (useful for tests or custom sinks).
func SetLogger(l *slog.Logger) {
	loggerMu.Lock()
	logger = l
	loggerMu.Unlock()
}

// DiscardLogging routes logs to /dev/null while preserving structured handler semantics.
func DiscardLogging() {
	SetLogger(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
}
