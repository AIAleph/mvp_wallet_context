package ch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHTTPStatusErrErrorString(t *testing.T) {
	err := &httpStatusErr{code: 503, body: "boom", op: "insert"}
	got := err.Error()
	want := "clickhouse insert http 503: boom"
	if got != want {
		t.Fatalf("unexpected error string: got %q want %q", got, want)
	}
}

func TestIsRetriableMatrix(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "http 429", err: &httpStatusErr{code: 429}, want: true},
		{name: "http 500", err: &httpStatusErr{code: 500}, want: true},
		{name: "http 499", err: &httpStatusErr{code: 499}, want: false},
		{name: "transport", err: errors.New("dial tcp"), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetriable(tc.err); got != tc.want {
				t.Fatalf("isRetriable(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDoWithRetryContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	oldAttempts := retryAttempts
	oldBackoff := retryBackoffBase
	retryAttempts = 3
	retryBackoffBase = time.Microsecond
	defer func() {
		retryAttempts = oldAttempts
		retryBackoffBase = oldBackoff
	}()

	err := doWithRetry(ctx, func() error { return errors.New("temporary") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestDoWithRetryExhaustsAttempts(t *testing.T) {
	oldAttempts := retryAttempts
	oldBackoff := retryBackoffBase
	retryAttempts = 2
	retryBackoffBase = time.Microsecond
	defer func() {
		retryAttempts = oldAttempts
		retryBackoffBase = oldBackoff
	}()

	want := &httpStatusErr{code: 503, body: "bad", op: "insert"}
	err := doWithRetry(context.Background(), func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("expected final error %v, got %v", want, err)
	}
}

func TestDoWithRetryImmediateSuccess(t *testing.T) {
	called := 0
	err := doWithRetry(context.Background(), func() error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if called != 1 {
		t.Fatalf("expected single call, got %d", called)
	}
}

func TestDoWithRetryNonRetriable(t *testing.T) {
	calls := 0
	ctx := context.Background()
	err := doWithRetry(ctx, func() error {
		calls++
		return &httpStatusErr{code: 400, body: "bad", op: "query"}
	})
	var statusErr *httpStatusErr
	if !errors.As(err, &statusErr) || statusErr.code != 400 {
		t.Fatalf("expected non-retriable status error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected single call for non-retriable error, got %d", calls)
	}
}

func TestDoWithRetryCanceledBetweenAttempts(t *testing.T) {
	oldAttempts := retryAttempts
	oldBackoff := retryBackoffBase
	retryAttempts = 3
	retryBackoffBase = 2 * time.Millisecond
	defer func() {
		retryAttempts = oldAttempts
		retryBackoffBase = oldBackoff
	}()

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	delay := retryBackoffBase / 2

	err := doWithRetry(ctx, func() error {
		attempts++
		if attempts == 1 {
			go func() {
				time.Sleep(delay)
				cancel()
			}()
			return &httpStatusErr{code: 503, body: "retry", op: "insert"}
		}
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected single attempt before cancellation, got %d", attempts)
	}
}
