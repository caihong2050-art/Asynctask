package asynctask

import "context"

// Executor defines how a task is executed.
// This allows swapping the execution strategy, e.g., using a worker pool,
// immediate execution, or a custom goroutine management strategy.
type Executor interface {
	// Execute runs the given function asynchronously.
	Execute(ctx context.Context, fn func())
}

// defaultExecutor simply launches a goroutine.
type defaultExecutor struct{}

func (e *defaultExecutor) Execute(_ context.Context, fn func()) {
	go fn()
}

// NewDefaultExecutor returns an executor that starts a new goroutine for each task.
func NewDefaultExecutor() Executor {
	return &defaultExecutor{}
}
