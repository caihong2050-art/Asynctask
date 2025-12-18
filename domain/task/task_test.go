package task

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	taskerrors "asynctask/domain/errors"
)

func TestNewTask(t *testing.T) {
	task := NewTask[string, int, string](
		"test",
		func(ctx context.Context, p string, pr ProgressReporter[int]) (string, error) {
			return "result: " + p, nil
		},
		WithName("test-task"),
		WithProgressBuffer(8),
	)

	if task.cfg.Name != "test-task" {
		t.Errorf("expected name 'test-task', got %s", task.cfg.Name)
	}
	if task.cfg.ProgressBufferSize != 8 {
		t.Errorf("expected buffer size 8, got %d", task.cfg.ProgressBufferSize)
	}
}

func TestTaskStart_Success(t *testing.T) {
	task := NewTask[string, int, string](
		"x",
		func(ctx context.Context, p string, pr ProgressReporter[int]) (string, error) {
			_ = pr.Report(1)
			_ = pr.Report(2)
			return "ok:" + p, nil
		},
		WithProgressBuffer(4),
	)

	h := task.Start(context.Background())

	var got []int
	for p := range h.Progress() {
		got = append(got, p)
	}

	res, err := h.Await(context.Background())
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if res != "ok:x" {
		t.Fatalf("unexpected result: %q", res)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("unexpected progress: %#v", got)
	}
	if h.State() != StateCompleted {
		t.Fatalf("expected state completed, got %s", h.State())
	}
}

func TestTaskStart_Cancel(t *testing.T) {
	task := NewTask[struct{}, struct{}, struct{}](
		struct{}{},
		func(ctx context.Context, _ struct{}, _ ProgressReporter[struct{}]) (struct{}, error) {
			select {
			case <-ctx.Done():
				return struct{}{}, ctx.Err()
			case <-time.After(2 * time.Second):
				return struct{}{}, nil
			}
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	h := task.Start(ctx)
	_, err := h.Await(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if h.State() != StateTimeout {
		t.Fatalf("expected state timeout, got %s", h.State())
	}
}

func TestTask_PanicRecovery(t *testing.T) {
	task := NewTask[int, struct{}, int](
		1,
		func(ctx context.Context, p int, _ ProgressReporter[struct{}]) (int, error) {
			panic("boom")
		},
	)

	h := task.Start(context.Background())
	_, err := h.Await(context.Background())
	if err == nil {
		t.Fatalf("expected panic converted to error")
	}
	var pe *taskerrors.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PanicError, got %T (%v)", err, err)
	}
	if h.State() != StateFailed {
		t.Fatalf("expected state failed, got %s", h.State())
	}
}

func TestTaskWithTimeout(t *testing.T) {
	task := NewTask[struct{}, struct{}, string](
		struct{}{},
		func(ctx context.Context, _ struct{}, _ ProgressReporter[struct{}]) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(1 * time.Second):
				return "done", nil
			}
		},
		WithTimeout(50*time.Millisecond),
	)

	h := task.Start(context.Background())
	_, err := h.Await(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestHandle_Cancel(t *testing.T) {
	started := make(chan struct{})
	task := NewTask[struct{}, struct{}, string](
		struct{}{},
		func(ctx context.Context, _ struct{}, _ ProgressReporter[struct{}]) (string, error) {
			close(started)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "done", nil
			}
		},
	)

	h := task.Start(context.Background())
	<-started
	h.Cancel()

	_, err := h.Await(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if h.State() != StateCancelled {
		t.Fatalf("expected state cancelled, got %s", h.State())
	}
}

func TestHandle_Result(t *testing.T) {
	task := NewTask[int, struct{}, int](
		42,
		func(ctx context.Context, p int, _ ProgressReporter[struct{}]) (int, error) {
			return p * 2, nil
		},
	)

	h := task.Start(context.Background())

	// Before completion
	_, _, ok := h.Result()
	// Note: the task might complete very fast, so we can't reliably test ok=false

	// Wait for completion
	<-h.Done()

	res, err, ok := h.Result()
	if !ok {
		t.Fatal("expected result to be available")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != 84 {
		t.Fatalf("expected 84, got %d", res)
	}
}

func TestHandle_Duration(t *testing.T) {
	task := NewTask[struct{}, struct{}, struct{}](
		struct{}{},
		func(ctx context.Context, _ struct{}, _ ProgressReporter[struct{}]) (struct{}, error) {
			time.Sleep(50 * time.Millisecond)
			return struct{}{}, nil
		},
	)

	h := task.Start(context.Background())
	<-h.Done()

	d := h.Duration()
	if d < 50*time.Millisecond {
		t.Fatalf("expected duration >= 50ms, got %v", d)
	}
}

func TestProgressReporter_NonBlocking(t *testing.T) {
	// Create task with small buffer
	task := NewTask[struct{}, int, struct{}](
		struct{}{},
		func(ctx context.Context, _ struct{}, pr ProgressReporter[int]) (struct{}, error) {
			// Send more progress than buffer size
			for i := 0; i < 100; i++ {
				pr.Report(i) // Should not block
			}
			return struct{}{}, nil
		},
		WithProgressBuffer(2),
	)

	h := task.Start(context.Background())
	_, err := h.Await(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventBus(t *testing.T) {
	eb := NewEventBus()
	var received int32

	eb.Subscribe(EventCompleted, func(e Event) {
		atomic.AddInt32(&received, 1)
	})

	eb.SubscribeAll(func(e Event) {
		atomic.AddInt32(&received, 1)
	})

	eb.Publish(Event{Type: EventCompleted})

	// Wait for async handlers
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&received) != 2 {
		t.Fatalf("expected 2 handler calls, got %d", received)
	}
}
