// Package service provides application-level services for task management.
package service

import (
	"context"
	"sync"
	"time"

	"asynctask/application/dto"
	"asynctask/domain/errors"
	"asynctask/domain/task"
)

// TaskService provides high-level task management operations.
type TaskService struct {
	mu       sync.RWMutex
	tasks    map[string]taskEntry
	eventBus *task.EventBus
	hooks    *HookManager
	startAt  time.Time

	// Statistics
	stats struct {
		completed int64
		failed    int64
	}
}

type taskEntry struct {
	handle    any
	state     task.State
	startTime time.Time
	config    task.Config
}

// NewTaskService creates a new TaskService.
func NewTaskService() *TaskService {
	return &TaskService{
		tasks:    make(map[string]taskEntry),
		eventBus: task.NewEventBus(),
		hooks:    NewHookManager(),
		startAt:  time.Now(),
	}
}

// EventBus returns the service's event bus.
func (s *TaskService) EventBus() *task.EventBus {
	return s.eventBus
}

// Hooks returns the service's hook manager.
func (s *TaskService) Hooks() *HookManager {
	return s.hooks
}

// Submit creates and starts a new task.
func (s *TaskService) Submit(
	ctx context.Context,
	fn task.Func[any, any, any],
	params any,
	req *dto.TaskCreateRequest,
) (string, error) {
	opts := req.ToTaskOptions()
	t := task.NewTask(params, fn, opts...)
	t.WithEventBus(s.eventBus)

	// Execute before-submit hooks
	if err := s.hooks.ExecuteBeforeSubmit(ctx, t.ID(), req); err != nil {
		return "", errors.NewTaskError(t.ID(), "pre-submit hook failed", err)
	}

	handle := t.Start(ctx)

	s.mu.Lock()
	s.tasks[t.ID()] = taskEntry{
		handle:    handle,
		state:     task.StateRunning,
		startTime: time.Now(),
		config:    t.Config(),
	}
	s.mu.Unlock()

	// Monitor task completion
	go s.monitorTask(t.ID(), handle)

	// Execute after-submit hooks
	s.hooks.ExecuteAfterSubmit(ctx, t.ID())

	return t.ID(), nil
}

// SubmitTyped creates and starts a typed task.
func SubmitTyped[Params, Progress, Result any](
	s *TaskService,
	ctx context.Context,
	fn task.Func[Params, Progress, Result],
	params Params,
	opts ...task.Option,
) (*task.Handle[Progress, Result], error) {
	t := task.NewTask(params, fn, opts...)
	t.WithEventBus(s.eventBus)

	handle := t.Start(ctx)

	s.mu.Lock()
	s.tasks[t.ID()] = taskEntry{
		handle:    handle,
		state:     task.StateRunning,
		startTime: time.Now(),
		config:    t.Config(),
	}
	s.mu.Unlock()

	// Monitor task completion
	go monitorTypedTask(s, t.ID(), handle)

	return handle, nil
}

func (s *TaskService) monitorTask(id string, handle *task.Handle[any, any]) {
	<-handle.Done()
	s.updateTaskCompletion(id, handle.State())
}

func monitorTypedTask[Progress, Result any](s *TaskService, id string, handle *task.Handle[Progress, Result]) {
	<-handle.Done()
	s.updateTaskCompletion(id, handle.State())
}

func (s *TaskService) updateTaskCompletion(id string, state task.State) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.tasks[id]; ok {
		entry.state = state
		s.tasks[id] = entry

		if state == task.StateCompleted {
			s.stats.completed++
		} else if state == task.StateFailed {
			s.stats.failed++
		}
	}
}

// Cancel cancels a running task.
func (s *TaskService) Cancel(ctx context.Context, taskID string) error {
	s.mu.RLock()
	entry, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if !ok {
		return errors.ErrTaskNotFound
	}

	if entry.state.IsTerminal() {
		return errors.ErrTaskNotRunning
	}

	// Type assertion to access Cancel method
	switch h := entry.handle.(type) {
	case interface{ Cancel() }:
		h.Cancel()
	default:
		return errors.NewTaskError(taskID, "cannot cancel", nil)
	}

	return nil
}

// GetStatus returns the status of a task.
func (s *TaskService) GetStatus(taskID string) (*dto.TaskStatus, error) {
	s.mu.RLock()
	entry, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if !ok {
		return nil, errors.ErrTaskNotFound
	}

	status := &dto.TaskStatus{
		ID:         taskID,
		Name:       entry.config.Name,
		State:      entry.state.String(),
		Priority:   entry.config.Priority.String(),
		Metadata:   entry.config.Metadata,
	}

	if !entry.startTime.IsZero() {
		status.StartTime = &entry.startTime
	}

	return status, nil
}

// ListTasks returns a list of tasks matching the criteria.
func (s *TaskService) ListTasks(req *dto.TaskListRequest) (*dto.TaskListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []dto.TaskStatus
	for id, entry := range s.tasks {
		// Apply filters
		if req.State != "" && entry.state.String() != req.State {
			continue
		}
		if req.Priority != "" && entry.config.Priority.String() != req.Priority {
			continue
		}

		status := dto.TaskStatus{
			ID:       id,
			Name:     entry.config.Name,
			State:    entry.state.String(),
			Priority: entry.config.Priority.String(),
			Metadata: entry.config.Metadata,
		}
		tasks = append(tasks, status)
	}

	// Apply pagination
	total := len(tasks)
	if req.Offset > 0 && req.Offset < len(tasks) {
		tasks = tasks[req.Offset:]
	}
	if req.Limit > 0 && req.Limit < len(tasks) {
		tasks = tasks[:req.Limit]
	}

	return &dto.TaskListResponse{
		Tasks:      tasks,
		TotalCount: total,
	}, nil
}

// GetStats returns executor statistics.
func (s *TaskService) GetStats() *dto.ExecutorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active, pending int
	for _, entry := range s.tasks {
		switch entry.state {
		case task.StateRunning:
			active++
		case task.StatePending, task.StateQueued:
			pending++
		}
	}

	return &dto.ExecutorStats{
		ActiveTasks:    active,
		PendingTasks:   pending,
		CompletedTasks: int(s.stats.completed),
		FailedTasks:    int(s.stats.failed),
		TotalTasks:     len(s.tasks),
		Uptime:         time.Since(s.startAt),
	}
}

// Cleanup removes completed tasks from memory.
func (s *TaskService) Cleanup(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, entry := range s.tasks {
		if entry.state.IsTerminal() && entry.startTime.Before(cutoff) {
			delete(s.tasks, id)
			removed++
		}
	}

	return removed
}

// HookManager manages task lifecycle hooks.
type HookManager struct {
	mu           sync.RWMutex
	beforeSubmit []BeforeSubmitHook
	afterSubmit  []AfterSubmitHook
	beforeExec   []BeforeExecHook
	afterExec    []AfterExecHook
}

// BeforeSubmitHook is called before a task is submitted.
type BeforeSubmitHook func(ctx context.Context, taskID string, req *dto.TaskCreateRequest) error

// AfterSubmitHook is called after a task is submitted.
type AfterSubmitHook func(ctx context.Context, taskID string)

// BeforeExecHook is called before task execution.
type BeforeExecHook func(ctx context.Context, taskID string) error

// AfterExecHook is called after task execution.
type AfterExecHook func(ctx context.Context, taskID string, err error)

// NewHookManager creates a new HookManager.
func NewHookManager() *HookManager {
	return &HookManager{}
}

// OnBeforeSubmit registers a before-submit hook.
func (hm *HookManager) OnBeforeSubmit(hook BeforeSubmitHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.beforeSubmit = append(hm.beforeSubmit, hook)
}

// OnAfterSubmit registers an after-submit hook.
func (hm *HookManager) OnAfterSubmit(hook AfterSubmitHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.afterSubmit = append(hm.afterSubmit, hook)
}

// OnBeforeExec registers a before-execution hook.
func (hm *HookManager) OnBeforeExec(hook BeforeExecHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.beforeExec = append(hm.beforeExec, hook)
}

// OnAfterExec registers an after-execution hook.
func (hm *HookManager) OnAfterExec(hook AfterExecHook) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.afterExec = append(hm.afterExec, hook)
}

// ExecuteBeforeSubmit runs all before-submit hooks.
func (hm *HookManager) ExecuteBeforeSubmit(ctx context.Context, taskID string, req *dto.TaskCreateRequest) error {
	hm.mu.RLock()
	hooks := hm.beforeSubmit
	hm.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx, taskID, req); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteAfterSubmit runs all after-submit hooks.
func (hm *HookManager) ExecuteAfterSubmit(ctx context.Context, taskID string) {
	hm.mu.RLock()
	hooks := hm.afterSubmit
	hm.mu.RUnlock()

	for _, hook := range hooks {
		hook(ctx, taskID)
	}
}

// ExecuteBeforeExec runs all before-execution hooks.
func (hm *HookManager) ExecuteBeforeExec(ctx context.Context, taskID string) error {
	hm.mu.RLock()
	hooks := hm.beforeExec
	hm.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx, taskID); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteAfterExec runs all after-execution hooks.
func (hm *HookManager) ExecuteAfterExec(ctx context.Context, taskID string, execErr error) {
	hm.mu.RLock()
	hooks := hm.afterExec
	hm.mu.RUnlock()

	for _, hook := range hooks {
		hook(ctx, taskID, execErr)
	}
}
