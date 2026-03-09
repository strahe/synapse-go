package retry

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"
)

// ErrMaxRetries is returned when all retry attempts are exhausted.
// The underlying cause is wrapped: errors.Is(err, ErrMaxRetries) == true.
var ErrMaxRetries = errors.New("max retries exceeded")

// Config holds retry parameters.
type Config struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	RetryIf      func(error) bool // nil = retry all non-context errors
}

// Option configures retry behavior.
type Option func(*Config)

// WithMaxRetries sets the maximum number of retries (not counting the initial attempt).
func WithMaxRetries(n int) Option {
	return func(c *Config) { c.MaxRetries = n }
}

// WithInitialDelay sets the base delay before the first retry.
func WithInitialDelay(d time.Duration) Option {
	return func(c *Config) { c.InitialDelay = d }
}

// WithMaxDelay sets the upper bound on backoff delay.
func WithMaxDelay(d time.Duration) Option {
	return func(c *Config) { c.MaxDelay = d }
}

// WithMultiplier sets the exponential backoff multiplier.
func WithMultiplier(m float64) Option {
	return func(c *Config) { c.Multiplier = m }
}

// WithRetryIf sets a predicate that determines whether an error is retryable.
// If the predicate returns false, the error is returned immediately without wrapping.
func WithRetryIf(fn func(error) bool) Option {
	return func(c *Config) { c.RetryIf = fn }
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:   3,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// Do executes fn with retry logic. It returns the result of the first
// successful call or ErrMaxRetries wrapping the last error.
func Do[T any](ctx context.Context, fn func(context.Context) (T, error), opts ...Option) (T, error) {
	cfg := DefaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	var zero T
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if cfg.RetryIf != nil && !cfg.RetryIf(err) {
			return zero, err
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return zero, ctxErr
			}
			return zero, err
		}

		if attempt == cfg.MaxRetries {
			break
		}

		backoff := jitteredBackoff(cfg.InitialDelay, cfg.MaxDelay, attempt, cfg.Multiplier)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return zero, fmt.Errorf("%w: %w", ErrMaxRetries, lastErr)
}

// jitteredBackoff computes an exponential backoff with decorrelated jitter.
func jitteredBackoff(base, max time.Duration, attempt int, multiplier float64) time.Duration {
	backoff := time.Duration(float64(base) * math.Pow(multiplier, float64(attempt)))
	if backoff > max {
		backoff = max
	}
	half := backoff / 2
	jitter := time.Duration(secureRandInt64n(int64(half) + 1))
	return half + jitter
}

// secureRandInt64n returns a cryptographically secure, bias-free random int64
// in [0, n). Goroutine-safe because crypto/rand is goroutine-safe.
func secureRandInt64n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	val, err := cryptorand.Int(cryptorand.Reader, big.NewInt(n))
	if err != nil {
		return 0
	}
	return val.Int64()
}
