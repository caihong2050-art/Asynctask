// Package executor provides a unified task execution engine.
package executor

import (
	"context"
	"sync"
	"time"

	"asynctask/domain/errors"
	"asynctask/domain/task"
	"asynctask/infrastructure/metrics"
	"asynctask/infrastructure/pool"
	"asynctask/infrastructure/scheduler"
	"asynctask/pkg/hooks"
	"asynctask/pkg/retry"
)

// Executor is the main entry point for task execution.
type Executor struct {
	mu sync.RWMutex

	pool      *pool.WorkerPool
	scheduler *scheduler.Scheduler
	eventBus  *task.EventBus
	hooks     *hooks.Registry
	metrics   *metrics.Collector

	config    Config
	startTime time.Time
	stopped   bool
}

// Config configures the executor.
type Config struct {
	// Worker pool settings
	MaxWorkers int
	QueueSize  int

	// Default task settings
	DefaultTimeout        time.Duration
	DefaultProgressBuffer int

	// Retry settings
	RetryStrategy retry.Strategy

	// Metrics
	EnableMetrics bool
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxWorkers:            10,
		QueueSize:             100,
		DefaultTimeout:        0, // No timeout
		DefaultProgressBuffer: 16,
		RetryStrategy:         retry.DefaultStrategy(),
		EnableMetrics:         true,
	}
}

// Option configures the executor.
type Option func(*Config)

// WithMaxWorkers sets the maximum number of workers.
func WithMaxWorkers(n int) Option {
	return func(c *Config) {
		c.MaxWorkers = n
	}
}

// WithQueueSize sets the job queue size.
func WithQueueSize(n int) Option {
	return func(c *Config) {
		c.QueueSize = n
	}
}

// WithDefaultTimeout sets the default task timeout.
func WithDefaultTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.DefaultTimeout = d
	}
}

// WithRetryStrategy sets the default retry strategy.
func WithRetryStrategy(s retry.Strategy) Option {
	return func(c *Config) {
		c.RetryStrategy = s
	}
}

// WithMetrics enables or disables metrics collection.
func WithMetrics(enabled bool) Option {
	return func(c *Config) {
		c.EnableMetrics = enabled
	}
}

// New creates a new Executor.
func New(opts ...Option) *Executor {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	eventBus := task.NewEventBus()
	hookRegistry := hooks.NewRegistry()
	metricsCollector := metrics.NewCollector()

	if cfg.EnableMetrics {
		metricsCollector.AttachToEventBus(eventBus)
	}

	workerPool := pool.NewWorkerPool(
		pool.WithMaxWorkers(cfg.MaxWorkers),
		pool.WithQueueSize(cfg.QueueSize),
	)

	sched := scheduler.NewScheduler(scheduler.SchedulerConfig{
		Pool:     workerPool,
		EventBus: eventBus,
	})

	return &Executor{
		pool:      workerPool,
		scheduler: sched,
		eventBus:  eventBus,
		hooks:     hookRegistry,
		metrics:   metricsCollector,
		config:    cfg,
		startTime: time.Now(),
	}
}

// Submit creates and starts a task immediately.
func Submit[Params, Progress, Result any](
	e *Executor,
	ctx context.Context,
	params Params,
	fn task.Func[Params, Progress, Result],
	opts ...task.Option,
) (*task.Handle[Progress, Result], error) {
	e.mu.RLock()
	if e.stopped {
		e.mu.RUnlock()
		return nil, errors.ErrExecutorShutdown
	}
	e.mu.RUnlock()

	// Apply default options
	allOpts := []task.Option{
		task.WithProgressBuffer(e.config.DefaultProgressBuffer),
	}
	if e.config.DefaultTimeout > 0 {
		allOpts = append(allOpts, task.WithTimeout(e.config.DefaultTimeout))
	}
	allOpts = append(allOpts, opts...)

	t := task.NewTask(params, fn, allOpts...)
	t.WithEventBus(e.eventBus)

	// Execute before-submit hooks
	hctx := &hooks.HookContext{
		TaskID:   t.ID(),
		Metadata: make(map[string]any),
	}
	if err := e.hooks.Execute(ctx, hooks.PhaseBeforeSubmit, hctx); err != nil {
		return nil, errors.NewTaskError(t.ID(), "pre-submit hook failed", err)
	}

	// Publish created event
	e.eventBus.Publish(task.Event{
		Type:      task.EventCreated,
		TaskID:    t.ID(),
		Timestamp: time.Now(),
	})

	handle := t.Start(ctx)

	// Execute after-submit hooks asynchronously
	go e.hooks.Execute(ctx, hooks.PhaseAfterSubmit, hctx)

	return handle, nil
}

// SubmitWithRetry creates and starts a task with retry support.
func SubmitWithRetry[Params, Progress, Result any](
	e *Executor,
	ctx context.Context,
	params Params,
	fn task.Func[Params, Progress, Result],
	strategy retry.Strategy,
	opts ...task.Option,
) (*task.Handle[Progress, Result], error) {
	if strategy == nil {
		strategy = e.config.RetryStrategy
	}

	// Wrap the function with retry logic
	wrappedFn := func(ctx context.Context, p Params, pr task.ProgressReporter[Progress]) (Result, error) {
		var result Result
		retrier := retry.NewRetrier(strategy.Clone())

		retrier.OnRetry = func(attempt int, err error, delay time.Duration) {
			e.eventBus.Publish(task.Event{
				Type:      task.EventRetry,
				Timestamp: time.Now(),
				Data:      &task.ErrorData{Error: err, RetryCount: attempt, WillRetry: true},
			})
		}

		err := retrier.Do(ctx, func(ctx context.Context) error {
			var execErr error
			result, execErr = fn(ctx, p, pr)
			return execErr
		})

		return result, err
	}

	return Submit(e, ctx, params, wrappedFn, opts...)
}

// Schedule adds a task to be executed at a specific time.
func Schedule[Params, Progress, Result any](
	e *Executor,
	ctx context.Context,
	at time.Time,
	priority task.Priority,
	params Params,
	fn task.Func[Params, Progress, Result],
	opts ...task.Option,
) (string, error) {
	e.mu.RLock()
	if e.stopped {
		e.mu.RUnlock()
		return "", errors.ErrExecutorShutdown
	}
	e.mu.RUnlock()

	allOpts := append([]task.Option{task.WithPriority(priority)}, opts...)
	t := task.NewTask(params, fn, allOpts...)
	taskID := t.ID()

	job := func(ctx context.Context) error {
		t.WithEventBus(e.eventBus)
		handle := t.Start(ctx)
		<-handle.Done()
		_, err := handle.Await(ctx)
		return err
	}

	e.scheduler.ScheduleAt(taskID, priority, at, job)

	return taskID, nil
}

// ScheduleAfter schedules a task to run after a delay.
func ScheduleAfter[Params, Progress, Result any](
	e *Executor,
	ctx context.Context,
	delay time.Duration,
	priority task.Priority,
	params Params,
	fn task.Func[Params, Progress, Result],
	opts ...task.Option,
) (string, error) {
	return Schedule(e, ctx, time.Now().Add(delay), priority, params, fn, opts...)
}

// EventBus returns the executor's event bus.
func (e *Executor) EventBus() *task.EventBus {
	return e.eventBus
}

// Hooks returns the executor's hook registry.
func (e *Executor) Hooks() *hooks.Registry {
	return e.hooks
}

// Metrics returns the executor's metrics collector.
func (e *Executor) Metrics() *metrics.Collector {
	return e.metrics
}

// Stats returns executor statistics.
func (e *Executor) Stats() Stats {
	poolStats := e.pool.Stats()
	schedStats := e.scheduler.Stats()
	metricsSnap := e.metrics.Snapshot()

	return Stats{
		Workers:     poolStats.Workers,
		Active:      poolStats.Active,
		Queued:      poolStats.Queued,
		Pending:     schedStats.Pending,
		Completed:   metricsSnap.TasksCompleted,
		Failed:      metricsSnap.TasksFailed,
		Cancelled:   metricsSnap.TasksCancelled,
		Uptime:      time.Since(e.startTime),
		SuccessRate: metricsSnap.SuccessRate(),
	}
}

// Stats contains executor statistics.
type Stats struct {
	Workers     int           `json:"workers"`
	Active      int           `json:"active"`
	Queued      int           `json:"queued"`
	Pending     int           `json:"pending"`
	Completed   int64         `json:"completed"`
	Failed      int64         `json:"failed"`
	Cancelled   int64         `json:"cancelled"`
	Uptime      time.Duration `json:"uptime"`
	SuccessRate float64       `json:"success_rate"`
}

// Shutdown gracefully stops the executor.
func (e *Executor) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return nil
	}
	e.stopped = true
	e.mu.Unlock()

	// Stop scheduler first
	e.scheduler.Stop()

	// Then stop worker pool
	done := make(chan struct{})
	go func() {
		e.pool.StopWait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// Stop immediately stops the executor without waiting.
func (e *Executor) Stop() {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	e.mu.Unlock()

	e.scheduler.Stop()
	e.pool.Stop()
}
