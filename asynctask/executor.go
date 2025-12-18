package asynctask

import "errors"

// Executor is the infrastructure port for running task workloads.
//
// This keeps the domain function (Func) independent from *how* it is executed
// (goroutine, worker pool, bounded queue, etc.).
type Executor interface {
	// TryExecute schedules fn to run asynchronously.
	// It should return a non-nil error if the work cannot be accepted.
	TryExecute(fn func()) error
}

// GoExecutor executes work by spawning a goroutine.
type GoExecutor struct{}

func (GoExecutor) TryExecute(fn func()) error {
	go fn()
	return nil
}

var ErrNilExecutor = errors.New("asynctask: nil executor")

