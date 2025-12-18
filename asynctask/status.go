package asynctask

import "fmt"

// State represents the current state of a task.
type State int

const (
	StatePending   State = iota // Task is created but not started
	StateRunning                // Task is currently running
	StateCompleted              // Task completed successfully
	StateFailed                 // Task failed with an error
	StateCancelled              // Task was cancelled
)

func (s State) String() string {
	switch s {
	case StatePending:
		return "Pending"
	case StateRunning:
		return "Running"
	case StateCompleted:
		return "Completed"
	case StateFailed:
		return "Failed"
	case StateCancelled:
		return "Cancelled"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}
