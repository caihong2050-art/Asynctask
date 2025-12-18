package asynctask

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"sync"
)

// taskConfig holds configuration for the task.
// It is non-generic to allow easy usage of options.
type taskConfig struct {
	progressBuffer int
	executor       Executor
	// interceptors are stored as any, but must be of type Interceptor[P, Pr, R].
	// Checked at runtime in NewTask.
	interceptors []any
}

type Option func(*taskConfig)

// WithProgressBuffer sets the progress channel buffer size (default: 16).
// If n < 0, it will be treated as 0.
func WithProgressBuffer(n int) Option {
	return func(c *taskConfig) {
		if n < 0 {
			n = 0
		}
		c.progressBuffer = n
	}
}

// WithExecutor sets a custom executor for the task.
func WithExecutor(e Executor) Option {
	return func(c *taskConfig) {
		c.executor = e
	}
}

// WithMiddleware adds interceptors to the task.
// Note: The types P, Pr, R must match the Task's types, otherwise NewTask will panic.
func WithMiddleware[Params, Progress, Result any](interceptors ...Interceptor[Params, Progress, Result]) Option {
	return func(c *taskConfig) {
		for _, i := range interceptors {
			c.interceptors = append(c.interceptors, i)
		}
	}
}

func defaultTaskConfig() taskConfig {
	return taskConfig{
		progressBuffer: 16,
		executor:       NewDefaultExecutor(),
	}
}

type progressReporter[Progress any] struct {
	ctx  context.Context
	ch   chan Progress
}

func (r *progressReporter[Progress]) Context() context.Context { return r.ctx }

func (r *progressReporter[Progress]) Report(p Progress) bool {
	select {
	case <-r.ctx.Done():
		return false
	default:
	}

	// Production default: never block the worker when progress buffer is full.
	// If the receiver is slow, progress events are dropped.
	select {
	case <-r.ctx.Done():
		return false
	case r.ch <- p:
		return true
	default:
		return true
	}
}

// Handle represents a running (or finished) task.
//
// Read Progress() until Done() is closed; Progress() will be closed afterwards.
type Handle[Progress, Result any] struct {
	done     chan struct{}
	progress chan Progress

	mu     sync.Mutex
	state  State
	result Result
	err    error

	cancel context.CancelFunc
}

// Done is closed when the task finishes (success, error, or cancellation).
func (h *Handle[Progress, Result]) Done() <-chan struct{} { return h.done }

// Progress yields progress updates until the task finishes; the channel is closed afterwards.
func (h *Handle[Progress, Result]) Progress() <-chan Progress { return h.progress }

// Cancel requests cancellation.
func (h *Handle[Progress, Result]) Cancel() {
	if h.cancel != nil {
		h.cancel()
	}
}

// State returns the current state of the task.
func (h *Handle[Progress, Result]) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Await waits for completion or ctx cancellation.
// If ctx is canceled first, it returns ctx.Err().
func (h *Handle[Progress, Result]) Await(ctx context.Context) (Result, error) {
	select {
	case <-ctx.Done():
		var zero Result
		return zero, ctx.Err()
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.result, h.err
	}
}

// Result returns the final result if already completed; ok=false otherwise.
func (h *Handle[Progress, Result]) Result() (res Result, err error, ok bool) {
	select {
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.result, h.err, true
	default:
		var zeroR Result
		return zeroR, nil, false
	}
}

// Task defines an async computation with progress reporting.
//
// Task itself is reusable; each Start creates a new Handle.
type Task[Params, Progress, Result any] struct {
	params       Params
	fn           Func[Params, Progress, Result]
	cfg          taskConfig
	interceptors []Interceptor[Params, Progress, Result]
}

// NewTask constructs a task.
func NewTask[Params, Progress, Result any](params Params, fn Func[Params, Progress, Result], opts ...Option) *Task[Params, Progress, Result] {
	cfg := defaultTaskConfig()
	for _, o := range opts {
		o(&cfg)
	}

	// Validate and cast interceptors
	var typedInterceptors []Interceptor[Params, Progress, Result]
	for _, i := range cfg.interceptors {
		if typed, ok := i.(Interceptor[Params, Progress, Result]); ok {
			typedInterceptors = append(typedInterceptors, typed)
		} else {
			// Panic with a clear error message if types mismatch
			panic(fmt.Sprintf("asynctask: middleware type mismatch. Expected Interceptor[%T, %T, %T], got %T",
				new(Params), new(Progress), new(Result), i))
		}
	}

	return &Task[Params, Progress, Result]{
		params:       params,
		fn:           fn,
		cfg:          cfg,
		interceptors: typedInterceptors,
	}
}

// Start runs the task using the configured executor.
func (t *Task[Params, Progress, Result]) Start(ctx context.Context) *Handle[Progress, Result] {
	ctx2, cancel := context.WithCancel(ctx)
	h := &Handle[Progress, Result]{
		done:     make(chan struct{}),
		progress: make(chan Progress, t.cfg.progressBuffer),
		cancel:   cancel,
		state:    StatePending,
	}

	t.cfg.executor.Execute(ctx2, func() {
		t.run(ctx2, h)
	})
	return h
}

func (t *Task[Params, Progress, Result]) run(ctx context.Context, h *Handle[Progress, Result]) {
	// Update state to Running
	h.mu.Lock()
	if h.state == StatePending {
		h.state = StateRunning
	}
	h.mu.Unlock()

	reporter := &progressReporter[Progress]{ctx: ctx, ch: h.progress}

	var (
		res Result
		err error
	)

	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r, Stack: debug.Stack()}
		}

		h.mu.Lock()
		h.result = res
		h.err = err

		// Determine final state
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				h.state = StateCancelled
			} else {
				h.state = StateFailed
			}
		} else {
			h.state = StateCompleted
		}

		h.mu.Unlock()

		// Ensure channels are closed after completion.
		close(h.progress)
		close(h.done)
	}()

	// Apply middleware
	fn := t.fn
	if len(t.interceptors) > 0 {
		wrapped := chainInterceptors(t.interceptors)
		if wrapped != nil {
			res, err = wrapped(ctx, t.params, reporter, fn)
			return
		}
	}

	res, err = fn(ctx, t.params, reporter)
}
