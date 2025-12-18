// Package task provides the core task domain model.
package task

import (
	"fmt"
	"sync"

	"asynctask/domain/errors"
)

// State represents the lifecycle state of a task.
type State uint32

const (
	// StatePending indicates the task is waiting to be executed.
	StatePending State = iota
	// StateQueued indicates the task has been queued for execution.
	StateQueued
	// StateRunning indicates the task is currently executing.
	StateRunning
	// StatePaused indicates the task has been paused.
	StatePaused
	// StateCompleted indicates the task completed successfully.
	StateCompleted
	// StateFailed indicates the task failed with an error.
	StateFailed
	// StateCancelled indicates the task was cancelled.
	StateCancelled
	// StateTimeout indicates the task timed out.
	StateTimeout
)

// String returns a human-readable representation of the state.
func (s State) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateQueued:
		return "queued"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateCompleted:
		return "completed"
	case StateFailed:
		return "failed"
	case StateCancelled:
		return "cancelled"
	case StateTimeout:
		return "timeout"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// IsTerminal returns true if the state is a terminal state.
func (s State) IsTerminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled || s == StateTimeout
}

// IsActive returns true if the task is actively being processed.
func (s State) IsActive() bool {
	return s == StateRunning || s == StatePaused
}

// validTransitions defines allowed state transitions.
var validTransitions = map[State][]State{
	StatePending:   {StateQueued, StateCancelled},
	StateQueued:    {StateRunning, StateCancelled},
	StateRunning:   {StateCompleted, StateFailed, StateCancelled, StateTimeout, StatePaused},
	StatePaused:    {StateRunning, StateCancelled},
	StateCompleted: {},
	StateFailed:    {StatePending}, // Allow retry
	StateCancelled: {},
	StateTimeout:   {StatePending}, // Allow retry
}

// StateMachine manages task state transitions with thread safety.
type StateMachine struct {
	mu       sync.RWMutex
	current  State
	history  []StateTransition
	onChange func(from, to State)
}

// StateTransition represents a state change event.
type StateTransition struct {
	From      State
	To        State
	Timestamp int64 // Unix timestamp in nanoseconds
}

// NewStateMachine creates a new state machine starting in Pending state.
func NewStateMachine(opts ...StateMachineOption) *StateMachine {
	sm := &StateMachine{
		current: StatePending,
		history: make([]StateTransition, 0, 8),
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

// StateMachineOption configures a StateMachine.
type StateMachineOption func(*StateMachine)

// WithOnChange sets a callback for state changes.
func WithOnChange(fn func(from, to State)) StateMachineOption {
	return func(sm *StateMachine) {
		sm.onChange = fn
	}
}

// WithInitialState sets the initial state.
func WithInitialState(s State) StateMachineOption {
	return func(sm *StateMachine) {
		sm.current = s
	}
}

// Current returns the current state.
func (sm *StateMachine) Current() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// CanTransitionTo checks if a transition to the target state is valid.
func (sm *StateMachine) CanTransitionTo(target State) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.canTransitionTo(target)
}

func (sm *StateMachine) canTransitionTo(target State) bool {
	allowed := validTransitions[sm.current]
	for _, s := range allowed {
		if s == target {
			return true
		}
	}
	return false
}

// TransitionTo attempts to transition to the target state.
func (sm *StateMachine) TransitionTo(target State, timestamp int64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.canTransitionTo(target) {
		return fmt.Errorf("%w: cannot transition from %s to %s",
			errors.ErrInvalidState, sm.current, target)
	}

	from := sm.current
	sm.current = target
	sm.history = append(sm.history, StateTransition{
		From:      from,
		To:        target,
		Timestamp: timestamp,
	})

	if sm.onChange != nil {
		sm.onChange(from, target)
	}

	return nil
}

// History returns a copy of the state transition history.
func (sm *StateMachine) History() []StateTransition {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]StateTransition, len(sm.history))
	copy(result, sm.history)
	return result
}
