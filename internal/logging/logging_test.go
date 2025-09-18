package logging

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestLoggerDefaultNonNil(t *testing.T) {
	if Logger() == nil {
		t.Fatal("default logger should not be nil")
	}
}

func TestSetLoggerOverrides(t *testing.T) {
	prev := Logger()
	t.Cleanup(func() { SetLogger(prev) })

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	custom := slog.New(handler)
	SetLogger(custom)

	if got := Logger(); got != custom {
		t.Fatalf("Logger() mismatch; want %p got %p", custom, got)
	}

	Logger().Info("test")
	if buf.Len() == 0 {
		t.Fatal("expected log output to custom handler")
	}
}

func TestDiscardLoggingReplacesLogger(t *testing.T) {
	prev := Logger()
	t.Cleanup(func() { SetLogger(prev) })

	DiscardLogging()
	if Logger() == nil {
		t.Fatal("discard logger should still be non-nil")
	}
	if Logger() == prev {
		t.Fatal("discard logging should replace existing logger")
	}
}
