// Package pool provides a worker pool for concurrent task execution.
package pool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"asynctask/domain/errors"
)

// Job represents a unit of work to be executed.
type Job func(ctx context.Context) error

// WorkerPool manages a pool of workers for concurrent task execution.
type WorkerPool struct {
	maxWorkers   int
	jobQueue     chan jobWrapper
	workerCount  int32
	activeCount  int32
	completedCnt int64
	failedCnt    int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.RWMutex
	stopped  bool
	handlers PoolHandlers
}

type jobWrapper struct {
	job     Job
	ctx     context.Context
	errChan chan error
}

// PoolHandlers contains callback handlers for pool events.
type PoolHandlers struct {
	OnJobStart    func(workerID int)
	OnJobComplete func(workerID int, duration time.Duration)
	OnJobError    func(workerID int, err error)
	OnPanic       func(workerID int, r any)
}

// PoolConfig configures the worker pool.
type PoolConfig struct {
	MaxWorkers    int
	QueueSize     int
	Handlers      PoolHandlers
	IdleTimeout   time.Duration // Time before idle workers are stopped
}

// DefaultPoolConfig returns a default configuration.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxWorkers:  10,
		QueueSize:   100,
		IdleTimeout: 30 * time.Second,
	}
}

// PoolOption configures a WorkerPool.
type PoolOption func(*PoolConfig)

// WithMaxWorkers sets the maximum number of workers.
func WithMaxWorkers(n int) PoolOption {
	return func(c *PoolConfig) {
		if n > 0 {
			c.MaxWorkers = n
		}
	}
}

// WithQueueSize sets the job queue size.
func WithQueueSize(n int) PoolOption {
	return func(c *PoolConfig) {
		if n > 0 {
			c.QueueSize = n
		}
	}
}

// WithHandlers sets the pool handlers.
func WithHandlers(h PoolHandlers) PoolOption {
	return func(c *PoolConfig) {
		c.Handlers = h
	}
}

// WithIdleTimeout sets the idle timeout for workers.
func WithIdleTimeout(d time.Duration) PoolOption {
	return func(c *PoolConfig) {
		c.IdleTimeout = d
	}
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(opts ...PoolOption) *WorkerPool {
	cfg := DefaultPoolConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		maxWorkers: cfg.MaxWorkers,
		jobQueue:   make(chan jobWrapper, cfg.QueueSize),
		ctx:        ctx,
		cancel:     cancel,
		handlers:   cfg.Handlers,
	}

	// Start initial workers
	for i := 0; i < cfg.MaxWorkers; i++ {
		pool.startWorker(i)
	}

	return pool
}

func (p *WorkerPool) startWorker(id int) {
	p.wg.Add(1)
	atomic.AddInt32(&p.workerCount, 1)

	go func() {
		defer func() {
			p.wg.Done()
			atomic.AddInt32(&p.workerCount, -1)
		}()

		p.worker(id)
	}()
}

func (p *WorkerPool) worker(id int) {
	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobQueue:
			if !ok {
				return
			}
			p.executeJob(id, job)
		}
	}
}

func (p *WorkerPool) executeJob(workerID int, jw jobWrapper) {
	atomic.AddInt32(&p.activeCount, 1)
	defer atomic.AddInt32(&p.activeCount, -1)

	start := time.Now()

	if p.handlers.OnJobStart != nil {
		p.handlers.OnJobStart(workerID)
	}

	var execErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = &errors.PanicError{Value: r}
				if p.handlers.OnPanic != nil {
					p.handlers.OnPanic(workerID, r)
				}
			}
		}()

		execErr = jw.job(jw.ctx)
	}()

	duration := time.Since(start)

	if execErr != nil {
		atomic.AddInt64(&p.failedCnt, 1)
		if p.handlers.OnJobError != nil {
			p.handlers.OnJobError(workerID, execErr)
		}
	} else {
		atomic.AddInt64(&p.completedCnt, 1)
	}

	if p.handlers.OnJobComplete != nil {
		p.handlers.OnJobComplete(workerID, duration)
	}

	if jw.errChan != nil {
		jw.errChan <- execErr
		close(jw.errChan)
	}
}

// Submit adds a job to the queue. Returns error if pool is stopped or queue is full.
func (p *WorkerPool) Submit(ctx context.Context, job Job) error {
	return p.submitInternal(ctx, job, nil)
}

// SubmitWait adds a job and waits for completion.
func (p *WorkerPool) SubmitWait(ctx context.Context, job Job) error {
	errChan := make(chan error, 1)
	if err := p.submitInternal(ctx, job, errChan); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func (p *WorkerPool) submitInternal(ctx context.Context, job Job, errChan chan error) error {
	p.mu.RLock()
	stopped := p.stopped
	p.mu.RUnlock()

	if stopped {
		return errors.ErrExecutorShutdown
	}

	jw := jobWrapper{
		job:     job,
		ctx:     ctx,
		errChan: errChan,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return errors.ErrExecutorShutdown
	case p.jobQueue <- jw:
		return nil
	default:
		return errors.ErrPoolFull
	}
}

// TrySubmit attempts to submit a job without blocking.
func (p *WorkerPool) TrySubmit(ctx context.Context, job Job) bool {
	return p.submitInternal(ctx, job, nil) == nil
}

// Stats returns pool statistics.
func (p *WorkerPool) Stats() PoolStats {
	return PoolStats{
		Workers:   int(atomic.LoadInt32(&p.workerCount)),
		Active:    int(atomic.LoadInt32(&p.activeCount)),
		Queued:    len(p.jobQueue),
		Completed: atomic.LoadInt64(&p.completedCnt),
		Failed:    atomic.LoadInt64(&p.failedCnt),
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	Workers   int   `json:"workers"`
	Active    int   `json:"active"`
	Queued    int   `json:"queued"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
}

// Stop gracefully stops the pool.
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	p.cancel()
	close(p.jobQueue)
}

// StopWait stops the pool and waits for all workers to finish.
func (p *WorkerPool) StopWait() {
	p.Stop()
	p.wg.Wait()
}

// Resize adjusts the number of workers.
func (p *WorkerPool) Resize(n int) {
	if n < 1 {
		n = 1
	}

	current := int(atomic.LoadInt32(&p.workerCount))
	if n > current {
		// Add workers
		for i := current; i < n; i++ {
			p.startWorker(i)
		}
	}
	// Note: Reducing workers requires a more complex implementation
	// with worker termination signals. For simplicity, we only support
	// increasing the pool size dynamically.
	p.maxWorkers = n
}
