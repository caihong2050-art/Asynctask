// Package retry provides retry strategies for task execution.
package retry

import (
	"context"
	"math"
	"math/rand"
	"time"

	"asynctask/domain/errors"
)

// Strategy defines a retry strategy.
type Strategy interface {
	// NextDelay returns the delay before the next retry attempt.
	// Returns (delay, shouldRetry).
	NextDelay(attempt int) (time.Duration, bool)
	// MaxAttempts returns the maximum number of attempts.
	MaxAttempts() int
	// Clone creates a copy of the strategy.
	Clone() Strategy
}

// FixedStrategy retries with a fixed delay.
type FixedStrategy struct {
	Delay    time.Duration
	Attempts int
}

// NextDelay implements Strategy.
func (s *FixedStrategy) NextDelay(attempt int) (time.Duration, bool) {
	if attempt >= s.Attempts {
		return 0, false
	}
	return s.Delay, true
}

// MaxAttempts implements Strategy.
func (s *FixedStrategy) MaxAttempts() int {
	return s.Attempts
}

// Clone implements Strategy.
func (s *FixedStrategy) Clone() Strategy {
	return &FixedStrategy{Delay: s.Delay, Attempts: s.Attempts}
}

// ExponentialStrategy retries with exponential backoff.
type ExponentialStrategy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Attempts     int
	Jitter       bool // Add random jitter to delays
}

// NextDelay implements Strategy.
func (s *ExponentialStrategy) NextDelay(attempt int) (time.Duration, bool) {
	if attempt >= s.Attempts {
		return 0, false
	}

	delay := float64(s.InitialDelay) * math.Pow(s.Multiplier, float64(attempt))
	if delay > float64(s.MaxDelay) {
		delay = float64(s.MaxDelay)
	}

	if s.Jitter {
		// Add up to 25% jitter
		jitter := delay * 0.25 * rand.Float64()
		delay += jitter
	}

	return time.Duration(delay), true
}

// MaxAttempts implements Strategy.
func (s *ExponentialStrategy) MaxAttempts() int {
	return s.Attempts
}

// Clone implements Strategy.
func (s *ExponentialStrategy) Clone() Strategy {
	return &ExponentialStrategy{
		InitialDelay: s.InitialDelay,
		MaxDelay:     s.MaxDelay,
		Multiplier:   s.Multiplier,
		Attempts:     s.Attempts,
		Jitter:       s.Jitter,
	}
}

// LinearStrategy retries with linearly increasing delays.
type LinearStrategy struct {
	InitialDelay time.Duration
	Increment    time.Duration
	MaxDelay     time.Duration
	Attempts     int
}

// NextDelay implements Strategy.
func (s *LinearStrategy) NextDelay(attempt int) (time.Duration, bool) {
	if attempt >= s.Attempts {
		return 0, false
	}

	delay := s.InitialDelay + time.Duration(attempt)*s.Increment
	if delay > s.MaxDelay {
		delay = s.MaxDelay
	}

	return delay, true
}

// MaxAttempts implements Strategy.
func (s *LinearStrategy) MaxAttempts() int {
	return s.Attempts
}

// Clone implements Strategy.
func (s *LinearStrategy) Clone() Strategy {
	return &LinearStrategy{
		InitialDelay: s.InitialDelay,
		Increment:    s.Increment,
		MaxDelay:     s.MaxDelay,
		Attempts:     s.Attempts,
	}
}

// DefaultStrategy returns a sensible default retry strategy.
func DefaultStrategy() Strategy {
	return &ExponentialStrategy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Attempts:     3,
		Jitter:       true,
	}
}

// NoRetry returns a strategy that never retries.
func NoRetry() Strategy {
	return &FixedStrategy{Attempts: 1}
}

// Retrier executes a function with retries.
type Retrier struct {
	Strategy     Strategy
	OnRetry      func(attempt int, err error, delay time.Duration)
	ShouldRetry  func(err error) bool // Custom retry decision function
}

// NewRetrier creates a new Retrier with the given strategy.
func NewRetrier(strategy Strategy) *Retrier {
	return &Retrier{
		Strategy:    strategy,
		ShouldRetry: DefaultShouldRetry,
	}
}

// DefaultShouldRetry is the default retry decision function.
// It retries on all errors except context cancellation and panic errors.
func DefaultShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var pe *errors.PanicError
	if errors.As(err, &pe) {
		return false // Don't retry panics
	}
	return true
}

// Do executes the function with retries.
// Attempts are counted starting from 1 (initial attempt).
func (r *Retrier) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error
	maxAttempts := r.Strategy.MaxAttempts()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Execute the function
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		// Check if we should retry
		if !r.ShouldRetry(err) {
			return err
		}

		// Check if this was the last attempt
		if attempt >= maxAttempts {
			return errors.NewTaskError("", "retry exhausted", lastErr)
		}

		// Get the delay for next attempt (attempt-1 because NextDelay is 0-indexed)
		delay, _ := r.Strategy.NextDelay(attempt - 1)

		// Notify retry callback
		if r.OnRetry != nil {
			r.OnRetry(attempt, err, delay)
		}

		// Wait before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// DoWithResult executes a function that returns a result with retries.
func DoWithResult[T any](ctx context.Context, r *Retrier, fn func(ctx context.Context) (T, error)) (T, error) {
	var result T
	err := r.Do(ctx, func(ctx context.Context) error {
		var e error
		result, e = fn(ctx)
		return e
	})
	return result, err
}

// RetryableError marks an error as retryable.
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	var re *RetryableError
	return errors.As(err, &re)
}

// MarkRetryable wraps an error to mark it as retryable.
func MarkRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Err: err}
}
