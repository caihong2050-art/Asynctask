package asynctask

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

type taskConfig struct {
	progressBuffer int
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

func defaultTaskConfig() taskConfig {
	return taskConfig{progressBuffer: 16}
}

// ProgressReporter lets a task publish progress updates.
// Report returns false if the task context is done.
//
// Implementations should be safe for single-producer usage.
type ProgressReporter[Progress any] interface {
	Report(p Progress) bool
	Context() context.Context
}

type progressReporter[Progress any] struct {
	ctx  context.Context
	ch   chan Progress
}

func (r *progressReporter[Progress]) Context() context.Context { return r.ctx }

func (r *progressReporter[Progress]) Report(p Progress) (ok bool) {
	// Defensive: users may (accidentally) call Report from another goroutine
	// after the task completes and the progress channel is closed.
	// Sending on a closed channel would panic and crash the process; we convert
	// that to a "false" return instead.
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

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

// Func is the background function executed by a Task.
//
// Contract:
// - Respect ctx for cancellation.
// - Use progress.Report to publish progress updates (optional).
// - Return (result, nil) on success; (zero, err) on failure.
type Func[Params, Progress, Result any] func(ctx context.Context, params Params, progress ProgressReporter[Progress]) (Result, error)

// PanicError is returned when a task panics (the panic is recovered).
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("asynctask: panic: %v", e.Value)
}

// Handle represents a running (or finished) task.
//
// Progress() is closed when the task finishes; Done() is closed afterwards.
// Consumers can safely range over Progress() and/or select on Done().
type Handle[Progress, Result any] struct {
	done     chan struct{}
	progress chan Progress

	mu     sync.Mutex
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

// Await waits for completion or ctx cancellation.
// If ctx is canceled first, it returns ctx.Err().
func (h *Handle[Progress, Result]) Await(ctx context.Context) (Result, error) {
	select {
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.result, h.err
	case <-ctx.Done():
		// Prefer returning the task result if it already completed,
		// even if the provided ctx is canceled at the same time.
		select {
		case <-h.done:
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.result, h.err
		default:
			var zero Result
			return zero, ctx.Err()
		}
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
	params Params
	fn     Func[Params, Progress, Result]
	cfg    taskConfig
}

// NewTask constructs a task.
func NewTask[Params, Progress, Result any](params Params, fn Func[Params, Progress, Result], opts ...Option) *Task[Params, Progress, Result] {
	cfg := defaultTaskConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return &Task[Params, Progress, Result]{params: params, fn: fn, cfg: cfg}
}

// Start runs the task on a new goroutine.
func (t *Task[Params, Progress, Result]) Start(ctx context.Context) *Handle[Progress, Result] {
	h, _ := t.StartOn(ctx, GoExecutor{})
	return h
}

func (t *Task[Params, Progress, Result]) run(ctx context.Context, h *Handle[Progress, Result]) {
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
		h.mu.Unlock()

		// Ensure channels are closed after completion.
		close(h.progress)
		close(h.done)
	}()

	res, err = t.fn(ctx, t.params, reporter)
}

// StartOn runs the task using the provided executor (infrastructure).
//
// This is the extensibility seam: you can plug in a worker pool, a bounded queue,
// a custom scheduler, etc., without changing the domain function.
func (t *Task[Params, Progress, Result]) StartOn(ctx context.Context, exec Executor) (*Handle[Progress, Result], error) {
	ctx2, cancel := context.WithCancel(ctx)
	h := &Handle[Progress, Result]{
		done:     make(chan struct{}),
		progress: make(chan Progress, t.cfg.progressBuffer),
		cancel:   cancel,
	}

	if exec == nil {
		// Fail-fast with a completed handle (so callers don't block forever).
		cancel()
		h.mu.Lock()
		h.err = ErrNilExecutor
		h.mu.Unlock()
		close(h.progress)
		close(h.done)
		return h, ErrNilExecutor
	}

	if err := exec.TryExecute(func() { t.run(ctx2, h) }); err != nil {
		// Ensure we don't leak a derived context / handle when scheduling fails.
		cancel()
		h.mu.Lock()
		h.err = err
		h.mu.Unlock()
		close(h.progress)
		close(h.done)
		return h, err
	}

	return h, nil
}
