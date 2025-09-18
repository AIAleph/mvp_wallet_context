package ingest

import "testing"

func TestNewPanicsOnInvalidAddress(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid address")
		}
	}()
	_ = New("not-hex", Options{})
}

func TestNewWithProviderPanicsOnInvalidAddress(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid address")
		}
	}()
	_ = NewWithProvider("invalid", Options{}, stubCursorProvider{head: 0})
}
