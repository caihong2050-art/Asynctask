package pool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_Submit(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(2), WithQueueSize(10))
	defer pool.StopWait()

	var executed int32
	err := pool.Submit(context.Background(), func(ctx context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	})

	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 1 {
		t.Fatalf("expected 1 execution, got %d", executed)
	}
}

func TestWorkerPool_SubmitWait(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(2))
	defer pool.StopWait()

	err := pool.SubmitWait(context.Background(), func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Fatalf("submit wait failed: %v", err)
	}
}

func TestWorkerPool_SubmitWait_Error(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(2))
	defer pool.StopWait()

	expectedErr := errors.New("test error")
	err := pool.SubmitWait(context.Background(), func(ctx context.Context) error {
		return expectedErr
	})

	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestWorkerPool_Concurrent(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(4), WithQueueSize(100))
	defer pool.StopWait()

	var executed int32
	const numJobs = 20

	for i := 0; i < numJobs; i++ {
		err := pool.Submit(context.Background(), func(ctx context.Context) error {
			atomic.AddInt32(&executed, 1)
			time.Sleep(10 * time.Millisecond)
			return nil
		})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&executed) != numJobs {
		t.Fatalf("expected %d executions, got %d", numJobs, executed)
	}
}

func TestWorkerPool_Stats(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(2), WithQueueSize(10))
	defer pool.StopWait()

	stats := pool.Stats()
	if stats.Workers != 2 {
		t.Errorf("expected 2 workers, got %d", stats.Workers)
	}

	// Submit a job
	done := make(chan struct{})
	pool.Submit(context.Background(), func(ctx context.Context) error {
		<-done
		return nil
	})

	time.Sleep(10 * time.Millisecond)
	stats = pool.Stats()
	if stats.Active != 1 {
		t.Errorf("expected 1 active, got %d", stats.Active)
	}

	close(done)
	time.Sleep(10 * time.Millisecond)

	stats = pool.Stats()
	if stats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", stats.Completed)
	}
}

func TestWorkerPool_Handlers(t *testing.T) {
	var startCount, completeCount, errorCount int32

	pool := NewWorkerPool(
		WithMaxWorkers(2),
		WithHandlers(PoolHandlers{
			OnJobStart: func(workerID int) {
				atomic.AddInt32(&startCount, 1)
			},
			OnJobComplete: func(workerID int, duration time.Duration) {
				atomic.AddInt32(&completeCount, 1)
			},
			OnJobError: func(workerID int, err error) {
				atomic.AddInt32(&errorCount, 1)
			},
		}),
	)
	defer pool.StopWait()

	// Successful job
	pool.SubmitWait(context.Background(), func(ctx context.Context) error {
		return nil
	})

	// Failed job
	pool.SubmitWait(context.Background(), func(ctx context.Context) error {
		return errors.New("test error")
	})

	if atomic.LoadInt32(&startCount) != 2 {
		t.Errorf("expected 2 starts, got %d", startCount)
	}
	if atomic.LoadInt32(&completeCount) != 2 {
		t.Errorf("expected 2 completions, got %d", completeCount)
	}
	if atomic.LoadInt32(&errorCount) != 1 {
		t.Errorf("expected 1 error, got %d", errorCount)
	}
}

func TestWorkerPool_Shutdown(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(2))
	pool.Stop()

	err := pool.Submit(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error after shutdown")
	}
}
