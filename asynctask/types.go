package asynctask

import (
	"context"
	"fmt"
)

// ProgressReporter lets a task publish progress updates.
// Report returns false if the task context is done.
//
// Implementations should be safe for single-producer usage.
type ProgressReporter[Progress any] interface {
	Report(p Progress) bool
	Context() context.Context
}

// Func is the background function executed by a Task.
//
// Contract:
// - Respect ctx for cancellation.
// - Use progress.Report to publish progress updates (optional).
// - Return (result, nil) on success; (zero, err) on failure.
type Func[Params, Progress, Result any] func(ctx context.Context, params Params, progress ProgressReporter[Progress]) (Result, error)

// PanicError is returned when a task panics (the panic is recovered).
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("asynctask: panic: %v", e.Value)
}
