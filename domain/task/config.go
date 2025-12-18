package task

import (
	"time"
)

// Priority represents task execution priority.
type Priority uint8

const (
	// PriorityLow is low priority.
	PriorityLow Priority = iota
	// PriorityNormal is normal priority.
	PriorityNormal
	// PriorityHigh is high priority.
	PriorityHigh
	// PriorityCritical is critical priority.
	PriorityCritical
)

// String returns a human-readable representation of the priority.
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Config holds task configuration options.
type Config struct {
	// ID is the unique identifier for the task.
	ID string

	// Name is a human-readable name for the task.
	Name string

	// Priority determines execution order.
	Priority Priority

	// Timeout is the maximum execution time (0 = no timeout).
	Timeout time.Duration

	// ProgressBufferSize is the size of the progress channel buffer.
	ProgressBufferSize int

	// Retry configuration
	RetryEnabled   bool
	MaxRetries     int
	RetryDelay     time.Duration
	RetryMaxDelay  time.Duration
	RetryBackoff   float64 // Multiplier for exponential backoff

	// Metadata for custom tags/labels.
	Metadata map[string]string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Priority:           PriorityNormal,
		Timeout:            0, // No timeout by default
		ProgressBufferSize: 16,
		RetryEnabled:       false,
		MaxRetries:         3,
		RetryDelay:         100 * time.Millisecond,
		RetryMaxDelay:      10 * time.Second,
		RetryBackoff:       2.0,
		Metadata:           make(map[string]string),
	}
}

// Option is a function that modifies Config.
type Option func(*Config)

// WithID sets the task ID.
func WithID(id string) Option {
	return func(c *Config) {
		c.ID = id
	}
}

// WithName sets the task name.
func WithName(name string) Option {
	return func(c *Config) {
		c.Name = name
	}
}

// WithPriority sets the task priority.
func WithPriority(p Priority) Option {
	return func(c *Config) {
		c.Priority = p
	}
}

// WithTimeout sets the task timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithProgressBuffer sets the progress channel buffer size.
func WithProgressBuffer(n int) Option {
	return func(c *Config) {
		if n < 0 {
			n = 0
		}
		c.ProgressBufferSize = n
	}
}

// WithRetry enables retry with specified parameters.
func WithRetry(maxRetries int, delay time.Duration) Option {
	return func(c *Config) {
		c.RetryEnabled = true
		c.MaxRetries = maxRetries
		c.RetryDelay = delay
	}
}

// WithRetryBackoff sets exponential backoff parameters.
func WithRetryBackoff(backoff float64, maxDelay time.Duration) Option {
	return func(c *Config) {
		c.RetryBackoff = backoff
		c.RetryMaxDelay = maxDelay
	}
}

// WithMetadata sets custom metadata.
func WithMetadata(key, value string) Option {
	return func(c *Config) {
		if c.Metadata == nil {
			c.Metadata = make(map[string]string)
		}
		c.Metadata[key] = value
	}
}

// Validate checks if the config is valid.
func (c *Config) Validate() error {
	if c.ProgressBufferSize < 0 {
		c.ProgressBufferSize = 0
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RetryBackoff < 1 {
		c.RetryBackoff = 1
	}
	return nil
}
