package task

import (
	"testing"
	"time"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StatePending, "pending"},
		{StateQueued, "queued"},
		{StateRunning, "running"},
		{StatePaused, "paused"},
		{StateCompleted, "completed"},
		{StateFailed, "failed"},
		{StateCancelled, "cancelled"},
		{StateTimeout, "timeout"},
		{State(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestState_IsTerminal(t *testing.T) {
	terminal := []State{StateCompleted, StateFailed, StateCancelled, StateTimeout}
	nonTerminal := []State{StatePending, StateQueued, StateRunning, StatePaused}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("State %s should be terminal", s)
		}
	}

	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("State %s should not be terminal", s)
		}
	}
}

func TestState_IsActive(t *testing.T) {
	active := []State{StateRunning, StatePaused}
	inactive := []State{StatePending, StateQueued, StateCompleted, StateFailed}

	for _, s := range active {
		if !s.IsActive() {
			t.Errorf("State %s should be active", s)
		}
	}

	for _, s := range inactive {
		if s.IsActive() {
			t.Errorf("State %s should not be active", s)
		}
	}
}

func TestStateMachine_Transitions(t *testing.T) {
	sm := NewStateMachine()

	if sm.Current() != StatePending {
		t.Fatalf("expected initial state pending, got %s", sm.Current())
	}

	// Valid transitions
	now := time.Now().UnixNano()
	if err := sm.TransitionTo(StateQueued, now); err != nil {
		t.Fatalf("transition to queued failed: %v", err)
	}
	if err := sm.TransitionTo(StateRunning, now); err != nil {
		t.Fatalf("transition to running failed: %v", err)
	}
	if err := sm.TransitionTo(StateCompleted, now); err != nil {
		t.Fatalf("transition to completed failed: %v", err)
	}

	// Invalid transition from terminal state
	if err := sm.TransitionTo(StateRunning, now); err == nil {
		t.Fatal("expected error for invalid transition from completed")
	}
}

func TestStateMachine_CanTransitionTo(t *testing.T) {
	sm := NewStateMachine()

	if !sm.CanTransitionTo(StateQueued) {
		t.Error("should be able to transition from pending to queued")
	}
	if !sm.CanTransitionTo(StateCancelled) {
		t.Error("should be able to transition from pending to cancelled")
	}
	if sm.CanTransitionTo(StateRunning) {
		t.Error("should not be able to transition from pending to running directly")
	}
}

func TestStateMachine_History(t *testing.T) {
	sm := NewStateMachine()
	now := time.Now().UnixNano()

	sm.TransitionTo(StateQueued, now)
	sm.TransitionTo(StateRunning, now+1)
	sm.TransitionTo(StateCompleted, now+2)

	history := sm.History()
	if len(history) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(history))
	}

	if history[0].From != StatePending || history[0].To != StateQueued {
		t.Errorf("unexpected first transition: %+v", history[0])
	}
	if history[1].From != StateQueued || history[1].To != StateRunning {
		t.Errorf("unexpected second transition: %+v", history[1])
	}
	if history[2].From != StateRunning || history[2].To != StateCompleted {
		t.Errorf("unexpected third transition: %+v", history[2])
	}
}

func TestStateMachine_OnChange(t *testing.T) {
	var changes []struct{ from, to State }

	sm := NewStateMachine(WithOnChange(func(from, to State) {
		changes = append(changes, struct{ from, to State }{from, to})
	}))

	now := time.Now().UnixNano()
	sm.TransitionTo(StateQueued, now)
	sm.TransitionTo(StateRunning, now)

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
}

func TestStateMachine_WithInitialState(t *testing.T) {
	sm := NewStateMachine(WithInitialState(StateRunning))

	if sm.Current() != StateRunning {
		t.Fatalf("expected initial state running, got %s", sm.Current())
	}
}
