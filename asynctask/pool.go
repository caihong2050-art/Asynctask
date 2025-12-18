package asynctask

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrExecutorClosed indicates the executor is closed and won't accept new work.
	ErrExecutorClosed = errors.New("asynctask: executor closed")
	// ErrQueueFull indicates the executor queue is full and cannot accept new work.
	ErrQueueFull = errors.New("asynctask: executor queue full")
)

// Pool is a bounded worker-pool executor.
//
// This is an infrastructure adapter implementing Executor.
// It is useful for production deployments to cap concurrency and queue depth.
type Pool struct {
	tasks chan func()

	closed atomic.Bool
	wg     sync.WaitGroup

	// cancel cancels the base context for workers.
	cancel context.CancelFunc
}

// NewPool creates a pool with the given number of workers and queue capacity.
//
// - workers <= 0 will be treated as 1.
// - queueCapacity < 0 will be treated as 0.
func NewPool(workers int, queueCapacity int) *Pool {
	if workers <= 0 {
		workers = 1
	}
	if queueCapacity < 0 {
		queueCapacity = 0
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		tasks:  make(chan func(), queueCapacity),
		cancel: cancel,
	}

	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker(ctx)
	}

	return p
}

func (p *Pool) worker(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case fn, ok := <-p.tasks:
			if !ok {
				return
			}
			if fn != nil {
				fn()
			}
		}
	}
}

// TryExecute attempts to enqueue work without blocking.
func (p *Pool) TryExecute(fn func()) error {
	if p == nil {
		return ErrNilExecutor
	}
	if fn == nil {
		return nil
	}
	if p.closed.Load() {
		return ErrExecutorClosed
	}

	select {
	case p.tasks <- fn:
		return nil
	default:
		return ErrQueueFull
	}
}

// Close stops accepting new work and waits for workers to exit.
//
// Close is idempotent.
func (p *Pool) Close() {
	if p == nil {
		return
	}
	if p.closed.Swap(true) {
		return
	}

	// Stop workers and close queue.
	p.cancel()
	close(p.tasks)

	// Wait for all workers to exit.
	p.wg.Wait()
}

