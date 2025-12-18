package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFixedStrategy(t *testing.T) {
	s := &FixedStrategy{
		Delay:    100 * time.Millisecond,
		Attempts: 3,
	}

	delay, ok := s.NextDelay(0)
	if !ok || delay != 100*time.Millisecond {
		t.Errorf("attempt 0: expected (100ms, true), got (%v, %v)", delay, ok)
	}

	delay, ok = s.NextDelay(2)
	if !ok || delay != 100*time.Millisecond {
		t.Errorf("attempt 2: expected (100ms, true), got (%v, %v)", delay, ok)
	}

	_, ok = s.NextDelay(3)
	if ok {
		t.Error("attempt 3: expected no retry")
	}
}

func TestExponentialStrategy(t *testing.T) {
	s := &ExponentialStrategy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Attempts:     5,
		Jitter:       false,
	}

	expected := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}

	for i, exp := range expected {
		delay, ok := s.NextDelay(i)
		if !ok {
			t.Errorf("attempt %d: expected retry", i)
			continue
		}
		if delay != exp {
			t.Errorf("attempt %d: expected %v, got %v", i, exp, delay)
		}
	}
}

func TestExponentialStrategy_MaxDelay(t *testing.T) {
	s := &ExponentialStrategy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   10.0,
		Attempts:     5,
		Jitter:       false,
	}

	delay, _ := s.NextDelay(2) // Would be 10000ms without cap
	if delay > s.MaxDelay {
		t.Errorf("delay %v exceeds max %v", delay, s.MaxDelay)
	}
}

func TestLinearStrategy(t *testing.T) {
	s := &LinearStrategy{
		InitialDelay: 100 * time.Millisecond,
		Increment:    50 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Attempts:     10,
	}

	delay0, _ := s.NextDelay(0)
	if delay0 != 100*time.Millisecond {
		t.Errorf("attempt 0: expected 100ms, got %v", delay0)
	}

	delay2, _ := s.NextDelay(2)
	if delay2 != 200*time.Millisecond {
		t.Errorf("attempt 2: expected 200ms, got %v", delay2)
	}

	// Test max delay cap
	delay10, _ := s.NextDelay(9)
	if delay10 > s.MaxDelay {
		t.Errorf("delay %v exceeds max %v", delay10, s.MaxDelay)
	}
}

func TestRetrier_Do_Success(t *testing.T) {
	r := NewRetrier(&FixedStrategy{Delay: 10 * time.Millisecond, Attempts: 3})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetrier_Do_SuccessAfterRetry(t *testing.T) {
	r := NewRetrier(&FixedStrategy{Delay: 10 * time.Millisecond, Attempts: 5})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetrier_Do_ExhaustedRetries(t *testing.T) {
	r := NewRetrier(&FixedStrategy{Delay: 10 * time.Millisecond, Attempts: 3})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("persistent error")
	})

	if err == nil {
		t.Error("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetrier_Do_ContextCancellation(t *testing.T) {
	r := NewRetrier(&FixedStrategy{Delay: 1 * time.Second, Attempts: 10})

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		return errors.New("error")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetrier_OnRetry(t *testing.T) {
	r := NewRetrier(&FixedStrategy{Delay: 10 * time.Millisecond, Attempts: 3})

	retryCount := 0
	r.OnRetry = func(attempt int, err error, delay time.Duration) {
		retryCount++
	}

	r.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("error")
	})

	if retryCount != 2 { // 2 retries (not counting initial attempt)
		t.Errorf("expected 2 retry callbacks, got %d", retryCount)
	}
}

func TestDoWithResult(t *testing.T) {
	r := NewRetrier(&FixedStrategy{Delay: 10 * time.Millisecond, Attempts: 3})

	result, err := DoWithResult(context.Background(), r, func(ctx context.Context) (int, error) {
		return 42, nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestNoRetry(t *testing.T) {
	s := NoRetry()
	if s.MaxAttempts() != 1 {
		t.Errorf("expected 1 attempt, got %d", s.MaxAttempts())
	}
}
