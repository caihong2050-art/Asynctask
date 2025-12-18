// Package metrics provides observability for the asynctask framework.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"asynctask/domain/task"
)

// Collector collects and exposes metrics for tasks.
type Collector struct {
	mu sync.RWMutex

	// Counters
	tasksCreated   int64
	tasksCompleted int64
	tasksFailed    int64
	tasksCancelled int64
	tasksTimeout   int64
	tasksRetried   int64

	// Gauges
	tasksActive int64

	// Histograms (simplified)
	durations []time.Duration

	// Labels
	byPriority map[task.Priority]*priorityMetrics
}

type priorityMetrics struct {
	created   int64
	completed int64
	failed    int64
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		durations:  make([]time.Duration, 0, 1000),
		byPriority: make(map[task.Priority]*priorityMetrics),
	}
}

// AttachToEventBus subscribes to task events for automatic metrics collection.
func (c *Collector) AttachToEventBus(eb *task.EventBus) {
	eb.SubscribeAll(c.handleEvent)
}

func (c *Collector) handleEvent(e task.Event) {
	switch e.Type {
	case task.EventCreated:
		atomic.AddInt64(&c.tasksCreated, 1)
		atomic.AddInt64(&c.tasksActive, 1)

	case task.EventCompleted:
		atomic.AddInt64(&c.tasksCompleted, 1)
		atomic.AddInt64(&c.tasksActive, -1)

	case task.EventFailed:
		atomic.AddInt64(&c.tasksFailed, 1)
		atomic.AddInt64(&c.tasksActive, -1)

	case task.EventCancelled:
		atomic.AddInt64(&c.tasksCancelled, 1)
		atomic.AddInt64(&c.tasksActive, -1)

	case task.EventTimeout:
		atomic.AddInt64(&c.tasksTimeout, 1)
		atomic.AddInt64(&c.tasksActive, -1)

	case task.EventRetry:
		atomic.AddInt64(&c.tasksRetried, 1)
	}
}

// RecordDuration records a task execution duration.
func (c *Collector) RecordDuration(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep last 10000 durations for percentile calculation
	if len(c.durations) >= 10000 {
		c.durations = c.durations[1:]
	}
	c.durations = append(c.durations, d)
}

// RecordByPriority records a task event by priority.
func (c *Collector) RecordByPriority(priority task.Priority, eventType task.EventType) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pm, ok := c.byPriority[priority]
	if !ok {
		pm = &priorityMetrics{}
		c.byPriority[priority] = pm
	}

	switch eventType {
	case task.EventCreated:
		pm.created++
	case task.EventCompleted:
		pm.completed++
	case task.EventFailed:
		pm.failed++
	}
}

// Snapshot returns a snapshot of current metrics.
func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	s := Snapshot{
		TasksCreated:   atomic.LoadInt64(&c.tasksCreated),
		TasksCompleted: atomic.LoadInt64(&c.tasksCompleted),
		TasksFailed:    atomic.LoadInt64(&c.tasksFailed),
		TasksCancelled: atomic.LoadInt64(&c.tasksCancelled),
		TasksTimeout:   atomic.LoadInt64(&c.tasksTimeout),
		TasksRetried:   atomic.LoadInt64(&c.tasksRetried),
		TasksActive:    atomic.LoadInt64(&c.tasksActive),
		Timestamp:      time.Now(),
	}

	// Calculate duration percentiles
	if len(c.durations) > 0 {
		s.DurationP50 = c.percentile(0.50)
		s.DurationP90 = c.percentile(0.90)
		s.DurationP99 = c.percentile(0.99)
	}

	// Copy priority metrics
	s.ByPriority = make(map[string]PrioritySnapshot)
	for p, pm := range c.byPriority {
		s.ByPriority[p.String()] = PrioritySnapshot{
			Created:   pm.created,
			Completed: pm.completed,
			Failed:    pm.failed,
		}
	}

	return s
}

// percentile calculates the p-th percentile of durations.
// Assumes c.mu is held.
func (c *Collector) percentile(p float64) time.Duration {
	if len(c.durations) == 0 {
		return 0
	}

	// Simple approach: sort and pick
	sorted := make([]time.Duration, len(c.durations))
	copy(sorted, c.durations)

	// Insertion sort for small slices, or use built-in for larger
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// Snapshot represents a point-in-time view of metrics.
type Snapshot struct {
	TasksCreated   int64 `json:"tasks_created"`
	TasksCompleted int64 `json:"tasks_completed"`
	TasksFailed    int64 `json:"tasks_failed"`
	TasksCancelled int64 `json:"tasks_cancelled"`
	TasksTimeout   int64 `json:"tasks_timeout"`
	TasksRetried   int64 `json:"tasks_retried"`
	TasksActive    int64 `json:"tasks_active"`

	DurationP50 time.Duration `json:"duration_p50"`
	DurationP90 time.Duration `json:"duration_p90"`
	DurationP99 time.Duration `json:"duration_p99"`

	ByPriority map[string]PrioritySnapshot `json:"by_priority,omitempty"`
	Timestamp  time.Time                   `json:"timestamp"`
}

// PrioritySnapshot contains metrics for a specific priority.
type PrioritySnapshot struct {
	Created   int64 `json:"created"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
}

// SuccessRate calculates the success rate.
func (s Snapshot) SuccessRate() float64 {
	total := s.TasksCompleted + s.TasksFailed
	if total == 0 {
		return 0
	}
	return float64(s.TasksCompleted) / float64(total)
}

// Reset clears all metrics.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	atomic.StoreInt64(&c.tasksCreated, 0)
	atomic.StoreInt64(&c.tasksCompleted, 0)
	atomic.StoreInt64(&c.tasksFailed, 0)
	atomic.StoreInt64(&c.tasksCancelled, 0)
	atomic.StoreInt64(&c.tasksTimeout, 0)
	atomic.StoreInt64(&c.tasksRetried, 0)
	atomic.StoreInt64(&c.tasksActive, 0)
	c.durations = c.durations[:0]
	c.byPriority = make(map[task.Priority]*priorityMetrics)
}

// Exporter defines an interface for metrics export.
type Exporter interface {
	Export(Snapshot) error
}

// LogExporter exports metrics to a logger.
type LogExporter struct {
	Logger func(format string, args ...any)
}

// Export implements Exporter.
func (e *LogExporter) Export(s Snapshot) error {
	e.Logger(
		"[metrics] created=%d completed=%d failed=%d cancelled=%d timeout=%d active=%d p50=%s p90=%s p99=%s success_rate=%.2f%%",
		s.TasksCreated, s.TasksCompleted, s.TasksFailed, s.TasksCancelled,
		s.TasksTimeout, s.TasksActive,
		s.DurationP50, s.DurationP90, s.DurationP99,
		s.SuccessRate()*100,
	)
	return nil
}
