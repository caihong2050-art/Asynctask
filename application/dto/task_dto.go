// Package dto provides Data Transfer Objects for the application layer.
package dto

import (
	"time"

	"asynctask/domain/task"
)

// TaskStatus represents a task's status information.
type TaskStatus struct {
	ID          string            `json:"id"`
	Name        string            `json:"name,omitempty"`
	State       string            `json:"state"`
	Priority    string            `json:"priority"`
	Progress    *ProgressInfo     `json:"progress,omitempty"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Duration    time.Duration     `json:"duration,omitempty"`
	RetryCount  int               `json:"retry_count"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ProgressInfo contains progress details.
type ProgressInfo struct {
	Current int64  `json:"current"`
	Total   int64  `json:"total"`
	Percent int    `json:"percent"`
	Message string `json:"message,omitempty"`
}

// TaskCreateRequest represents a task creation request.
type TaskCreateRequest struct {
	Name           string            `json:"name,omitempty"`
	Priority       string            `json:"priority,omitempty"`
	Timeout        time.Duration     `json:"timeout,omitempty"`
	RetryEnabled   bool              `json:"retry_enabled,omitempty"`
	MaxRetries     int               `json:"max_retries,omitempty"`
	RetryDelay     time.Duration     `json:"retry_delay,omitempty"`
	ProgressBuffer int               `json:"progress_buffer,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// TaskCreateResponse represents a task creation response.
type TaskCreateResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// TaskListRequest represents a task list request.
type TaskListRequest struct {
	State    string `json:"state,omitempty"`
	Priority string `json:"priority,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// TaskListResponse represents a task list response.
type TaskListResponse struct {
	Tasks      []TaskStatus `json:"tasks"`
	TotalCount int          `json:"total_count"`
}

// TaskEventDTO represents a task event for external consumption.
type TaskEventDTO struct {
	Type      string    `json:"type"`
	TaskID    string    `json:"task_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data,omitempty"`
}

// ExecutorStats represents executor statistics.
type ExecutorStats struct {
	ActiveTasks   int           `json:"active_tasks"`
	PendingTasks  int           `json:"pending_tasks"`
	CompletedTasks int          `json:"completed_tasks"`
	FailedTasks   int           `json:"failed_tasks"`
	TotalTasks    int           `json:"total_tasks"`
	WorkerCount   int           `json:"worker_count"`
	Uptime        time.Duration `json:"uptime"`
}

// ToTaskOptions converts a TaskCreateRequest to task.Option slice.
func (r *TaskCreateRequest) ToTaskOptions() []task.Option {
	var opts []task.Option

	if r.Name != "" {
		opts = append(opts, task.WithName(r.Name))
	}

	if r.Priority != "" {
		p := parsePriority(r.Priority)
		opts = append(opts, task.WithPriority(p))
	}

	if r.Timeout > 0 {
		opts = append(opts, task.WithTimeout(r.Timeout))
	}

	if r.ProgressBuffer > 0 {
		opts = append(opts, task.WithProgressBuffer(r.ProgressBuffer))
	}

	if r.RetryEnabled {
		opts = append(opts, task.WithRetry(r.MaxRetries, r.RetryDelay))
	}

	for k, v := range r.Metadata {
		opts = append(opts, task.WithMetadata(k, v))
	}

	return opts
}

// parsePriority converts a string to task.Priority.
func parsePriority(s string) task.Priority {
	switch s {
	case "low":
		return task.PriorityLow
	case "normal":
		return task.PriorityNormal
	case "high":
		return task.PriorityHigh
	case "critical":
		return task.PriorityCritical
	default:
		return task.PriorityNormal
	}
}

// FromTaskEvent converts a task.Event to TaskEventDTO.
func FromTaskEvent(e task.Event) TaskEventDTO {
	return TaskEventDTO{
		Type:      e.Type.String(),
		TaskID:    e.TaskID,
		Timestamp: e.Timestamp,
		Data:      e.Data,
	}
}
