package asynctask

import "errors"

var (
	// ErrRunnerClosed is returned when submitting to a closed Runner.
	ErrRunnerClosed = errors.New("asynctask: runner closed")
)
