package asynctask

import (
	"context"
	"fmt"
	"sync"
)

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
	cfg  taskConfig
	ch   chan Progress
	once sync.Once
}

func (r *progressReporter[Progress]) Context() context.Context { return r.ctx }

func (r *progressReporter[Progress]) Report(p Progress) bool {
	select {
	case <-r.ctx.Done():
		return false
	default:
	}

	switch r.cfg.progressMode {
	case ProgressBlock:
		select {
		case <-r.ctx.Done():
			return false
		case r.ch <- p:
			return true
		}
	case ProgressDropIfFull:
		select {
		case <-r.ctx.Done():
			return false
		case r.ch <- p:
			return true
		default:
			return true
		}
	default:
		// Fallback: drop.
		select {
		case r.ch <- p:
			return true
		default:
			return true
		}
	}
}

// Func is the background function executed by a Task.
//
// Contract:
// - Respect ctx for cancellation.
// - Use progress.Report to publish progress updates (optional).
// - Return (result, nil) on success; (zero, err) on failure.
type Func[Params, Progress, Result any] func(ctx context.Context, params Params, progress ProgressReporter[Progress]) (Result, error)

// Handle represents a running (or finished) task.
//
// Read Progress() until Done() is closed; Progress() will be closed afterwards.
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
// Task itself is reusable; each Start/Submit creates a new Handle.
type Task[Params, Progress, Result any] struct {
	params Params
	fn     Func[Params, Progress, Result]
	cfg    taskConfig
}

// NewTask constructs a task.
func NewTask[Params, Progress, Result any](params Params, fn Func[Params, Progress, Result], opts ...TaskOption) *Task[Params, Progress, Result] {
	cfg := defaultTaskConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return &Task[Params, Progress, Result]{params: params, fn: fn, cfg: cfg}
}

// Start runs the task on a new goroutine.
func (t *Task[Params, Progress, Result]) Start(ctx context.Context) *Handle[Progress, Result] {
	ctx2, cancel := context.WithCancel(ctx)
	h := &Handle[Progress, Result]{
		done:     make(chan struct{}),
		progress: make(chan Progress, t.cfg.progressBuffer),
		cancel:   cancel,
	}

	go t.run(ctx2, h)
	return h
}

// Submit runs the task on a Runner worker pool.
func (t *Task[Params, Progress, Result]) Submit(ctx context.Context, r *Runner) (*Handle[Progress, Result], error) {
	ctx2, cancel := context.WithCancel(ctx)
	h := &Handle[Progress, Result]{
		done:     make(chan struct{}),
		progress: make(chan Progress, t.cfg.progressBuffer),
		cancel:   cancel,
	}

	j := taskJob[Params, Progress, Result]{t: t, ctx: ctx2, h: h}
	if err := r.Submit(j); err != nil {
		cancel()
		close(h.progress)
		close(h.done)
		return nil, err
	}
	return h, nil
}

type taskJob[Params, Progress, Result any] struct {
	t   *Task[Params, Progress, Result]
	ctx context.Context
	h   *Handle[Progress, Result]
}

func (j taskJob[Params, Progress, Result]) run() { j.t.run(j.ctx, j.h) }

func (t *Task[Params, Progress, Result]) run(ctx context.Context, h *Handle[Progress, Result]) {
	reporter := &progressReporter[Progress]{ctx: ctx, cfg: t.cfg, ch: h.progress}

	defer func() {
		// Ensure progress channel is closed after completion.
		close(h.progress)
		close(h.done)
	}()

	if t.cfg.recoverPanics {
		defer func() {
			if r := recover(); r != nil {
				h.mu.Lock()
				defer h.mu.Unlock()
				h.err = fmt.Errorf("asynctask: panic: %v", r)
			}
		}()
	}

	res, err := t.fn(ctx, t.params, reporter)

	h.mu.Lock()
	h.result = res
	h.err = err
	h.mu.Unlock()
}
