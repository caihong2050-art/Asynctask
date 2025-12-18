package asynctask

type ProgressMode int

const (
	// ProgressDropIfFull drops progress events when the progress buffer is full.
	ProgressDropIfFull ProgressMode = iota
	// ProgressBlock blocks when the progress buffer is full.
	ProgressBlock
)

type taskConfig struct {
	progressBuffer int
	progressMode   ProgressMode
	recoverPanics  bool
}

type TaskOption func(*taskConfig)

// WithProgressBuffer sets the progress channel buffer size.
func WithProgressBuffer(n int) TaskOption {
	return func(c *taskConfig) {
		if n < 0 {
			n = 0
		}
		c.progressBuffer = n
	}
}

// WithProgressMode sets how progress reporting behaves when the buffer is full.
func WithProgressMode(m ProgressMode) TaskOption {
	return func(c *taskConfig) {
		c.progressMode = m
	}
}

// WithPanicRecovery converts panics inside task execution to an error.
func WithPanicRecovery(enabled bool) TaskOption {
	return func(c *taskConfig) {
		c.recoverPanics = enabled
	}
}

func defaultTaskConfig() taskConfig {
	return taskConfig{
		progressBuffer: 16,
		progressMode:   ProgressDropIfFull,
		recoverPanics:  true,
	}
}
