// Package errors defines domain-specific errors for the asynctask framework.
package errors

import (
	"errors"
	"fmt"
)

var (
	// ErrTaskNotFound indicates the task was not found.
	ErrTaskNotFound = errors.New("asynctask: task not found")

	// ErrTaskAlreadyStarted indicates the task has already been started.
	ErrTaskAlreadyStarted = errors.New("asynctask: task already started")

	// ErrTaskNotRunning indicates the task is not in running state.
	ErrTaskNotRunning = errors.New("asynctask: task not running")

	// ErrTaskCancelled indicates the task was cancelled.
	ErrTaskCancelled = errors.New("asynctask: task cancelled")

	// ErrTaskTimeout indicates the task timed out.
	ErrTaskTimeout = errors.New("asynctask: task timeout")

	// ErrExecutorShutdown indicates the executor has been shut down.
	ErrExecutorShutdown = errors.New("asynctask: executor shutdown")

	// ErrPoolFull indicates the worker pool is full.
	ErrPoolFull = errors.New("asynctask: worker pool full")

	// ErrInvalidState indicates an invalid state transition.
	ErrInvalidState = errors.New("asynctask: invalid state transition")

	// ErrRetryExhausted indicates all retry attempts have been exhausted.
	ErrRetryExhausted = errors.New("asynctask: retry attempts exhausted")
)

// PanicError wraps a panic value with stack trace information.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("asynctask: panic: %v", e.Value)
}

// TaskError wraps an error with task-specific context.
type TaskError struct {
	TaskID  string
	Message string
	Cause   error
}

func (e *TaskError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("asynctask: task %s: %s: %v", e.TaskID, e.Message, e.Cause)
	}
	return fmt.Sprintf("asynctask: task %s: %s", e.TaskID, e.Message)
}

func (e *TaskError) Unwrap() error {
	return e.Cause
}

// NewTaskError creates a new TaskError.
func NewTaskError(taskID, message string, cause error) *TaskError {
	return &TaskError{
		TaskID:  taskID,
		Message: message,
		Cause:   cause,
	}
}

// Is provides error matching support.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As provides error type assertion support.
func As(err error, target any) bool {
	return errors.As(err, target)
}
