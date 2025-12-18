package task

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"asynctask/domain/errors"
)

// ProgressReporter allows a task to publish progress updates.
type ProgressReporter[Progress any] interface {
	// Report sends a progress update. Returns false if the task context is done.
	Report(p Progress) bool
	// Context returns the task's context.
	Context() context.Context
}

type progressReporter[Progress any] struct {
	ctx context.Context
	ch  chan Progress
}

func (r *progressReporter[Progress]) Context() context.Context { return r.ctx }

func (r *progressReporter[Progress]) Report(p Progress) bool {
	select {
	case <-r.ctx.Done():
		return false
	default:
	}

	// Non-blocking send: drop progress if buffer is full.
	select {
	case <-r.ctx.Done():
		return false
	case r.ch <- p:
		return true
	default:
		return true // Progress dropped, but task continues
	}
}

// Func is the function signature for task execution.
type Func[Params, Progress, Result any] func(ctx context.Context, params Params, progress ProgressReporter[Progress]) (Result, error)

// Handle represents a running or completed task.
type Handle[Progress, Result any] struct {
	id       string
	done     chan struct{}
	progress chan Progress

	mu     sync.RWMutex
	result Result
	err    error
	state  *StateMachine

	cancel   context.CancelFunc
	eventBus *EventBus

	// Metrics
	startTime  time.Time
	endTime    time.Time
	retryCount int32
}

// ID returns the task ID.
func (h *Handle[Progress, Result]) ID() string { return h.id }

// Done is closed when the task finishes.
func (h *Handle[Progress, Result]) Done() <-chan struct{} { return h.done }

// Progress yields progress updates until the task finishes.
func (h *Handle[Progress, Result]) Progress() <-chan Progress { return h.progress }

// State returns the current task state.
func (h *Handle[Progress, Result]) State() State {
	return h.state.Current()
}

// Cancel requests task cancellation.
func (h *Handle[Progress, Result]) Cancel() {
	if h.cancel != nil {
		h.cancel()
	}
}

// Await waits for completion or ctx cancellation.
func (h *Handle[Progress, Result]) Await(ctx context.Context) (Result, error) {
	select {
	case <-ctx.Done():
		var zero Result
		return zero, ctx.Err()
	case <-h.done:
		h.mu.RLock()
		defer h.mu.RUnlock()
		return h.result, h.err
	}
}

// Result returns the final result if completed; ok=false otherwise.
func (h *Handle[Progress, Result]) Result() (res Result, err error, ok bool) {
	select {
	case <-h.done:
		h.mu.RLock()
		defer h.mu.RUnlock()
		return h.result, h.err, true
	default:
		var zero Result
		return zero, nil, false
	}
}

// Duration returns the execution duration. Returns 0 if not started.
func (h *Handle[Progress, Result]) Duration() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.startTime.IsZero() {
		return 0
	}
	if h.endTime.IsZero() {
		return time.Since(h.startTime)
	}
	return h.endTime.Sub(h.startTime)
}

// RetryCount returns the number of retries.
func (h *Handle[Progress, Result]) RetryCount() int {
	return int(atomic.LoadInt32(&h.retryCount))
}

// publishEvent sends an event through the event bus.
func (h *Handle[Progress, Result]) publishEvent(eventType EventType, data any) {
	if h.eventBus != nil {
		h.eventBus.Publish(Event{
			Type:      eventType,
			TaskID:    h.id,
			Timestamp: time.Now(),
			Data:      data,
		})
	}
}

// Task defines an async computation with progress reporting.
type Task[Params, Progress, Result any] struct {
	params   Params
	fn       Func[Params, Progress, Result]
	cfg      Config
	eventBus *EventBus
}

// NewTask constructs a task with the given parameters and function.
func NewTask[Params, Progress, Result any](
	params Params,
	fn Func[Params, Progress, Result],
	opts ...Option,
) *Task[Params, Progress, Result] {
	cfg := DefaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	_ = cfg.Validate()

	// Generate ID if not provided
	if cfg.ID == "" {
		cfg.ID = generateID()
	}

	return &Task[Params, Progress, Result]{
		params: params,
		fn:     fn,
		cfg:    cfg,
	}
}

// ID returns the task ID.
func (t *Task[Params, Progress, Result]) ID() string {
	return t.cfg.ID
}

// Config returns a copy of the task configuration.
func (t *Task[Params, Progress, Result]) Config() Config {
	return t.cfg
}

// WithEventBus attaches an event bus to the task.
func (t *Task[Params, Progress, Result]) WithEventBus(eb *EventBus) *Task[Params, Progress, Result] {
	t.eventBus = eb
	return t
}

// Start runs the task on a new goroutine.
func (t *Task[Params, Progress, Result]) Start(ctx context.Context) *Handle[Progress, Result] {
	ctx2, cancel := context.WithCancel(ctx)

	// Apply timeout if configured
	if t.cfg.Timeout > 0 {
		ctx2, cancel = context.WithTimeout(ctx, t.cfg.Timeout)
	}

	h := &Handle[Progress, Result]{
		id:       t.cfg.ID,
		done:     make(chan struct{}),
		progress: make(chan Progress, t.cfg.ProgressBufferSize),
		state:    NewStateMachine(),
		cancel:   cancel,
		eventBus: t.eventBus,
	}

	go t.run(ctx2, h)
	return h
}

func (t *Task[Params, Progress, Result]) run(ctx context.Context, h *Handle[Progress, Result]) {
	reporter := &progressReporter[Progress]{ctx: ctx, ch: h.progress}

	var (
		res Result
		err error
	)

	// Transition to running state
	now := time.Now()
	_ = h.state.TransitionTo(StateQueued, now.UnixNano())
	_ = h.state.TransitionTo(StateRunning, now.UnixNano())

	h.mu.Lock()
	h.startTime = now
	h.mu.Unlock()

	h.publishEvent(EventStarted, nil)

	defer func() {
		if r := recover(); r != nil {
			err = &errors.PanicError{Value: r, Stack: debug.Stack()}
		}

		h.mu.Lock()
		h.result = res
		h.err = err
		h.endTime = time.Now()
		h.mu.Unlock()

		// Determine final state
		finalState := StateCompleted
		eventType := EventCompleted
		var eventData any

		if err != nil {
			if errors.Is(err, context.Canceled) {
				finalState = StateCancelled
				eventType = EventCancelled
			} else if errors.Is(err, context.DeadlineExceeded) {
				finalState = StateTimeout
				eventType = EventTimeout
			} else {
				finalState = StateFailed
				eventType = EventFailed
				eventData = &ErrorData{Error: err}
			}
		}

		_ = h.state.TransitionTo(finalState, h.endTime.UnixNano())
		h.publishEvent(eventType, eventData)

		close(h.progress)
		close(h.done)
	}()

	res, err = t.fn(ctx, t.params, reporter)
}

// generateID creates a simple unique ID.
func generateID() string {
	return time.Now().Format("20060102150405.000000000")
}

// TaskGroup manages a collection of tasks.
type TaskGroup struct {
	mu       sync.Mutex
	handles  []any
	eventBus *EventBus
}

// NewTaskGroup creates a new task group.
func NewTaskGroup() *TaskGroup {
	return &TaskGroup{
		handles:  make([]any, 0),
		eventBus: NewEventBus(),
	}
}

// EventBus returns the group's event bus.
func (tg *TaskGroup) EventBus() *EventBus {
	return tg.eventBus
}

// Add adds a handle to the group.
func (tg *TaskGroup) Add(h any) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.handles = append(tg.handles, h)
}

// Count returns the number of handles in the group.
func (tg *TaskGroup) Count() int {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	return len(tg.handles)
}
