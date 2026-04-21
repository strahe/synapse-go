package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo_Success(t *testing.T) {
	result, err := Do(context.Background(), func(_ context.Context) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("got %q, want %q", result, "ok")
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	var attempts int
	result, err := Do(context.Background(), func(_ context.Context) (int, error) {
		attempts++
		if attempts < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	}, WithInitialDelay(time.Millisecond), WithMaxRetries(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Fatalf("got %d, want 42", result)
	}
	if attempts != 3 {
		t.Fatalf("got %d attempts, want 3", attempts)
	}
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	permanent := errors.New("permanent")
	var attempts int
	_, err := Do(context.Background(), func(_ context.Context) (string, error) {
		attempts++
		return "", permanent
	}, WithMaxRetries(2), WithInitialDelay(time.Millisecond))

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrMaxRetries) {
		t.Fatalf("expected ErrMaxRetries, got: %v", err)
	}
	if !errors.Is(err, permanent) {
		t.Fatalf("expected wrapped permanent error, got: %v", err)
	}
	// 1 initial + 2 retries = 3
	if attempts != 3 {
		t.Fatalf("got %d attempts, want 3", attempts)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var attempts int32
	_, err := Do(ctx, func(_ context.Context) (string, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 2 {
			cancel()
		}
		return "", errors.New("fail")
	}, WithMaxRetries(10), WithInitialDelay(time.Millisecond))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestDo_DownstreamContextErrorDoesNotReturnNil(t *testing.T) {
	downstreamErr := context.DeadlineExceeded

	_, err := Do(context.Background(), func(_ context.Context) (string, error) {
		return "", downstreamErr
	}, WithMaxRetries(0))

	if err == nil {
		t.Fatal("expected downstream context error, got nil")
	}
	if !errors.Is(err, downstreamErr) {
		t.Fatalf("expected downstream context error, got: %v", err)
	}
}

func TestDo_RetryIf(t *testing.T) {
	retryable := errors.New("retryable")
	nonRetryable := errors.New("non-retryable")

	var attempts int
	_, err := Do(context.Background(), func(_ context.Context) (string, error) {
		attempts++
		if attempts == 1 {
			return "", retryable
		}
		return "", nonRetryable
	}, WithMaxRetries(5), WithInitialDelay(time.Millisecond),
		WithRetryIf(func(err error) bool {
			return errors.Is(err, retryable)
		}),
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should return the non-retryable error directly, not wrapped in ErrMaxRetries.
	if errors.Is(err, ErrMaxRetries) {
		t.Fatalf("should not be ErrMaxRetries, got: %v", err)
	}
	if !errors.Is(err, nonRetryable) {
		t.Fatalf("expected non-retryable error, got: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("got %d attempts, want 2", attempts)
	}
}

func TestDo_Options(t *testing.T) {
	cfg := DefaultConfig()
	opts := []Option{
		WithMaxRetries(5),
		WithInitialDelay(100 * time.Millisecond),
		WithMaxDelay(10 * time.Second),
		WithMultiplier(3.0),
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.MaxRetries != 5 {
		t.Fatalf("MaxRetries: got %d, want 5", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Fatalf("InitialDelay: got %v, want 100ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Fatalf("MaxDelay: got %v, want 10s", cfg.MaxDelay)
	}
	if cfg.Multiplier != 3.0 {
		t.Fatalf("Multiplier: got %f, want 3.0", cfg.Multiplier)
	}
}

func TestDo_BackoffRange(t *testing.T) {
	base := time.Millisecond
	maxD := 100 * time.Millisecond
	multiplier := 2.0

	for attempt := 0; attempt < 5; attempt++ {
		fullBackoff := time.Duration(float64(base) * pow(multiplier, attempt))
		if fullBackoff > maxD {
			fullBackoff = maxD
		}
		half := fullBackoff / 2

		for range 50 {
			got := jitteredBackoff(base, maxD, attempt, multiplier)
			if got < half || got > fullBackoff {
				t.Fatalf("attempt %d: backoff %v out of range [%v, %v]",
					attempt, got, half, fullBackoff)
			}
		}
	}
}

func TestDo_GenericTypes(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		v, err := Do(context.Background(), func(_ context.Context) (string, error) {
			return "hello", nil
		})
		if err != nil || v != "hello" {
			t.Fatalf("got (%q, %v), want (\"hello\", nil)", v, err)
		}
	})

	t.Run("int", func(t *testing.T) {
		v, err := Do(context.Background(), func(_ context.Context) (int, error) {
			return 99, nil
		})
		if err != nil || v != 99 {
			t.Fatalf("got (%d, %v), want (99, nil)", v, err)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type result struct {
			Name  string
			Count int
		}
		want := result{Name: "test", Count: 7}
		v, err := Do(context.Background(), func(_ context.Context) (result, error) {
			return want, nil
		})
		if err != nil || v != want {
			t.Fatalf("got (%+v, %v), want (%+v, nil)", v, err, want)
		}
	})
}

func pow(base float64, exp int) float64 {
	result := 1.0
	for range exp {
		result *= base
	}
	return result
}

func TestDo_MaxRetriesZero_NoRetries(t *testing.T) {
	var attempts int
	_, err := Do(context.Background(), func(_ context.Context) (string, error) {
		attempts++
		return "", errors.New("fail")
	}, WithMaxRetries(0), WithInitialDelay(time.Millisecond))
	if attempts != 1 {
		t.Fatalf("attempts=%d want 1 (no retries)", attempts)
	}
	if !errors.Is(err, ErrMaxRetries) {
		t.Fatalf("err=%v want wrapped ErrMaxRetries", err)
	}
}

func TestDo_PreCancelledContext_FirstCheckShortCircuits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var attempts int32
	_, err := Do(ctx, func(_ context.Context) (string, error) {
		atomic.AddInt32(&attempts, 1)
		return "ok", nil
	}, WithMaxRetries(3), WithInitialDelay(time.Millisecond))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	if n := atomic.LoadInt32(&attempts); n != 0 {
		t.Fatalf("attempts=%d want 0 (should not invoke fn)", n)
	}
}

func TestDo_ContextCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var attempts int32
	done := make(chan struct{})
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
		close(done)
	}()
	_, err := Do(ctx, func(_ context.Context) (string, error) {
		atomic.AddInt32(&attempts, 1)
		return "", errors.New("fail")
	}, WithMaxRetries(10), WithInitialDelay(100*time.Millisecond))
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	if n := atomic.LoadInt32(&attempts); n > 2 {
		t.Fatalf("attempts=%d want <=2 (cancel should interrupt backoff)", n)
	}
}

func TestDo_BackoffCappedByMaxDelay(t *testing.T) {
	const maxD = 50 * time.Millisecond
	for attempt := 0; attempt < 20; attempt++ {
		got := jitteredBackoff(time.Millisecond, maxD, attempt, 4.0)
		if got > maxD {
			t.Fatalf("attempt=%d backoff=%v exceeds max=%v", attempt, got, maxD)
		}
	}
}

func TestDo_RetryIfFalseReturnsImmediately(t *testing.T) {
	permanent := errors.New("do-not-retry")
	var attempts int
	_, err := Do(context.Background(), func(_ context.Context) (string, error) {
		attempts++
		return "", permanent
	}, WithMaxRetries(5), WithInitialDelay(time.Millisecond),
		WithRetryIf(func(e error) bool { return !errors.Is(e, permanent) }),
	)
	if attempts != 1 {
		t.Fatalf("attempts=%d want 1 (RetryIf false → no retry)", attempts)
	}
	if !errors.Is(err, permanent) {
		t.Fatalf("err=%v want permanent", err)
	}
	if errors.Is(err, ErrMaxRetries) {
		t.Fatal("err should not be ErrMaxRetries when RetryIf returns false")
	}
}
