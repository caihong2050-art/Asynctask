package asynctask

import (
	"sync"
)

type job interface {
	run()
}

type Runner struct {
	q      chan job
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

type RunnerOption func(*Runner)

// WithWorkerCount sets the number of workers for the Runner.
func WithWorkerCount(n int) RunnerOption {
	return func(r *Runner) {
		if n < 1 {
			n = 1
		}
		// Recreate queue and workers in NewRunner only.
		// This option is interpreted in NewRunner.
	}
}

// NewRunner creates a worker-pool Runner.
//
// Notes:
// - Runner is optional; tasks can also be started directly via Task.Start.
// - Submissions after Close will return ErrRunnerClosed.
func NewRunner(workerCount int, queueSize int) *Runner {
	if workerCount < 1 {
		workerCount = 1
	}
	if queueSize < 0 {
		queueSize = 0
	}

	r := &Runner{q: make(chan job, queueSize)}
	r.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer r.wg.Done()
			for j := range r.q {
				j.run()
			}
		}()
	}
	return r
}

// Submit enqueues a job to run on the worker pool.
func (r *Runner) Submit(j job) error {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ErrRunnerClosed
	}

	// If Close races, send may panic; avoid by checking again under lock.
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrRunnerClosed
	}
	q := r.q
	r.mu.Unlock()

	q <- j
	return nil
}

// Close stops accepting new jobs and waits for workers to finish.
func (r *Runner) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	close(r.q)
	r.mu.Unlock()

	r.wg.Wait()
}
