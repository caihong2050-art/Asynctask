package asynctask

import (
	"context"
	"errors"
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
}
