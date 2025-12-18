package asynctask

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

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
		t.Errorf("expected StateCompleted, got %v", h.State())
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
	if h.State() != StateCancelled {
		t.Errorf("expected StateCancelled, got %v", h.State())
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
	var pe *PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PanicError, got %T (%v)", err, err)
	}
	if h.State() != StateFailed {
		t.Errorf("expected StateFailed, got %v", h.State())
	}
}

func TestTask_Middleware(t *testing.T) {
	var log []string
	
	loggingMiddleware := func(name string) Interceptor[string, int, string] {
		return func(ctx context.Context, params string, r ProgressReporter[int], next Func[string, int, string]) (string, error) {
			log = append(log, fmt.Sprintf("%s: before", name))
			res, err := next(ctx, params, r)
			log = append(log, fmt.Sprintf("%s: after", name))
			return res, err
		}
	}

	task := NewTask[string, int, string](
		"world",
		func(ctx context.Context, p string, _ ProgressReporter[int]) (string, error) {
			log = append(log, "exec")
			return "hello " + p, nil
		},
		WithMiddleware(loggingMiddleware("m1"), loggingMiddleware("m2")),
	)

	h := task.Start(context.Background())
	res, _ := h.Await(context.Background())

	if res != "hello world" {
		t.Errorf("unexpected result: %s", res)
	}

	expectedLog := []string{"m1: before", "m2: before", "exec", "m2: after", "m1: after"}
	if len(log) != len(expectedLog) {
		t.Fatalf("log mismatch: got %v, want %v", log, expectedLog)
	}
	for i, v := range log {
		if v != expectedLog[i] {
			t.Errorf("log[%d]: got %s, want %s", i, v, expectedLog[i])
		}
	}
}

func TestTask_Executor(t *testing.T) {
	var executed int32
	
	// Custom executor that runs synchronously
	syncExec := &syncExecutor{}
	
	task := NewTask[struct{}, struct{}, struct{}](
		struct{}{},
		func(ctx context.Context, _ struct{}, _ ProgressReporter[struct{}]) (struct{}, error) {
			atomic.StoreInt32(&executed, 1)
			return struct{}{}, nil
		},
		WithExecutor(syncExec),
	)

	h := task.Start(context.Background())
	// Because it's sync, it should be done immediately
	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("task not executed immediately")
	}
	h.Await(context.Background())
}

type syncExecutor struct{}
func (e *syncExecutor) Execute(ctx context.Context, fn func()) {
	fn()
}
