// Package hooks provides a flexible hook system for task lifecycle events.
package hooks

import (
	"context"
	"sync"
)

// HookPhase represents when a hook should be executed.
type HookPhase uint8

const (
	// PhaseBeforeSubmit is called before task submission.
	PhaseBeforeSubmit HookPhase = iota
	// PhaseAfterSubmit is called after task submission.
	PhaseAfterSubmit
	// PhaseBeforeExecute is called before task execution.
	PhaseBeforeExecute
	// PhaseAfterExecute is called after task execution.
	PhaseAfterExecute
	// PhaseOnProgress is called on progress updates.
	PhaseOnProgress
	// PhaseOnError is called when an error occurs.
	PhaseOnError
	// PhaseOnRetry is called before a retry attempt.
	PhaseOnRetry
	// PhaseOnCancel is called when a task is cancelled.
	PhaseOnCancel
	// PhaseOnTimeout is called when a task times out.
	PhaseOnTimeout
)

// String returns a human-readable representation of the phase.
func (p HookPhase) String() string {
	switch p {
	case PhaseBeforeSubmit:
		return "before_submit"
	case PhaseAfterSubmit:
		return "after_submit"
	case PhaseBeforeExecute:
		return "before_execute"
	case PhaseAfterExecute:
		return "after_execute"
	case PhaseOnProgress:
		return "on_progress"
	case PhaseOnError:
		return "on_error"
	case PhaseOnRetry:
		return "on_retry"
	case PhaseOnCancel:
		return "on_cancel"
	case PhaseOnTimeout:
		return "on_timeout"
	default:
		return "unknown"
	}
}

// HookContext provides context to hook functions.
type HookContext struct {
	TaskID     string
	Phase      HookPhase
	Attempt    int
	Error      error
	Progress   any
	Metadata   map[string]any
}

// Hook is the function signature for hooks.
type Hook func(ctx context.Context, hctx *HookContext) error

// Priority determines the order of hook execution.
type Priority int

const (
	PriorityFirst  Priority = -1000
	PriorityNormal Priority = 0
	PriorityLast   Priority = 1000
)

type hookEntry struct {
	hook     Hook
	priority Priority
	name     string
}

// Registry manages hook registrations.
type Registry struct {
	mu    sync.RWMutex
	hooks map[HookPhase][]hookEntry
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[HookPhase][]hookEntry),
	}
}

// Register adds a hook for the specified phase.
func (r *Registry) Register(phase HookPhase, hook Hook, opts ...RegisterOption) {
	cfg := &registerConfig{
		priority: PriorityNormal,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	entry := hookEntry{
		hook:     hook,
		priority: cfg.priority,
		name:     cfg.name,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	hooks := r.hooks[phase]
	hooks = append(hooks, entry)

	// Sort by priority (lower values first)
	for i := len(hooks) - 1; i > 0; i-- {
		if hooks[i].priority < hooks[i-1].priority {
			hooks[i], hooks[i-1] = hooks[i-1], hooks[i]
		}
	}

	r.hooks[phase] = hooks
}

type registerConfig struct {
	priority Priority
	name     string
}

// RegisterOption configures hook registration.
type RegisterOption func(*registerConfig)

// WithPriority sets the hook priority.
func WithPriority(p Priority) RegisterOption {
	return func(c *registerConfig) {
		c.priority = p
	}
}

// WithName sets the hook name for debugging.
func WithName(name string) RegisterOption {
	return func(c *registerConfig) {
		c.name = name
	}
}

// Execute runs all hooks for the given phase.
func (r *Registry) Execute(ctx context.Context, phase HookPhase, hctx *HookContext) error {
	r.mu.RLock()
	hooks := r.hooks[phase]
	r.mu.RUnlock()

	hctx.Phase = phase

	for _, entry := range hooks {
		if err := entry.hook(ctx, hctx); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteAsync runs all hooks asynchronously and waits for completion.
func (r *Registry) ExecuteAsync(ctx context.Context, phase HookPhase, hctx *HookContext) []error {
	r.mu.RLock()
	hooks := r.hooks[phase]
	r.mu.RUnlock()

	hctx.Phase = phase
	errs := make([]error, len(hooks))
	var wg sync.WaitGroup

	for i, entry := range hooks {
		wg.Add(1)
		go func(idx int, h Hook) {
			defer wg.Done()
			errs[idx] = h(ctx, hctx)
		}(i, entry.hook)
	}

	wg.Wait()

	// Filter out nil errors
	var result []error
	for _, err := range errs {
		if err != nil {
			result = append(result, err)
		}
	}

	return result
}

// Unregister removes a named hook.
func (r *Registry) Unregister(phase HookPhase, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	hooks := r.hooks[phase]
	for i, entry := range hooks {
		if entry.name == name {
			r.hooks[phase] = append(hooks[:i], hooks[i+1:]...)
			return true
		}
	}
	return false
}

// Clear removes all hooks for a phase (or all phases if no phase specified).
func (r *Registry) Clear(phases ...HookPhase) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(phases) == 0 {
		r.hooks = make(map[HookPhase][]hookEntry)
		return
	}

	for _, phase := range phases {
		delete(r.hooks, phase)
	}
}

// Count returns the number of registered hooks.
func (r *Registry) Count(phase HookPhase) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hooks[phase])
}

// Common hook implementations

// LoggingHook creates a hook that logs events.
func LoggingHook(logger func(format string, args ...any)) Hook {
	return func(ctx context.Context, hctx *HookContext) error {
		logger("[hook] task=%s phase=%s attempt=%d", hctx.TaskID, hctx.Phase, hctx.Attempt)
		return nil
	}
}

// MetricsHook creates a hook that records metrics.
func MetricsHook(recorder func(taskID string, phase HookPhase)) Hook {
	return func(ctx context.Context, hctx *HookContext) error {
		recorder(hctx.TaskID, hctx.Phase)
		return nil
	}
}

// ValidationHook creates a hook that validates task context.
func ValidationHook(validator func(*HookContext) error) Hook {
	return func(ctx context.Context, hctx *HookContext) error {
		return validator(hctx)
	}
}

// ChainHooks combines multiple hooks into a single hook.
func ChainHooks(hooks ...Hook) Hook {
	return func(ctx context.Context, hctx *HookContext) error {
		for _, h := range hooks {
			if err := h(ctx, hctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// ConditionalHook executes a hook only if the condition is met.
func ConditionalHook(condition func(*HookContext) bool, hook Hook) Hook {
	return func(ctx context.Context, hctx *HookContext) error {
		if condition(hctx) {
			return hook(ctx, hctx)
		}
		return nil
	}
}
