// Package scheduler provides task scheduling capabilities.
package scheduler

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"asynctask/domain/task"
	"asynctask/infrastructure/pool"
)

// ScheduledTask represents a task scheduled for execution.
type ScheduledTask struct {
	ID        string
	Priority  task.Priority
	ScheduleAt time.Time
	Job       pool.Job
	index     int // Used by heap
}

// priorityQueue implements heap.Interface for scheduled tasks.
type priorityQueue []*ScheduledTask

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority first, then earlier schedule time
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	return pq[i].ScheduleAt.Before(pq[j].ScheduleAt)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*ScheduledTask)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// Scheduler manages task scheduling with priority support.
type Scheduler struct {
	mu         sync.Mutex
	queue      priorityQueue
	pool       *pool.WorkerPool
	ctx        context.Context
	cancel     context.CancelFunc
	notify     chan struct{}
	eventBus   *task.EventBus
}

// SchedulerConfig configures the scheduler.
type SchedulerConfig struct {
	Pool     *pool.WorkerPool
	EventBus *task.EventBus
}

// NewScheduler creates a new scheduler.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		queue:    make(priorityQueue, 0),
		pool:     cfg.Pool,
		ctx:      ctx,
		cancel:   cancel,
		notify:   make(chan struct{}, 1),
		eventBus: cfg.EventBus,
	}

	heap.Init(&s.queue)
	go s.run()

	return s
}

// Schedule adds a task to the schedule.
func (s *Scheduler) Schedule(st *ScheduledTask) {
	s.mu.Lock()
	heap.Push(&s.queue, st)
	s.mu.Unlock()

	// Notify the scheduler
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// ScheduleImmediate schedules a task for immediate execution.
func (s *Scheduler) ScheduleImmediate(id string, priority task.Priority, job pool.Job) {
	s.Schedule(&ScheduledTask{
		ID:         id,
		Priority:   priority,
		ScheduleAt: time.Now(),
		Job:        job,
	})
}

// ScheduleAt schedules a task for a specific time.
func (s *Scheduler) ScheduleAt(id string, priority task.Priority, at time.Time, job pool.Job) {
	s.Schedule(&ScheduledTask{
		ID:         id,
		Priority:   priority,
		ScheduleAt: at,
		Job:        job,
	})
}

// ScheduleAfter schedules a task after a delay.
func (s *Scheduler) ScheduleAfter(id string, priority task.Priority, delay time.Duration, job pool.Job) {
	s.ScheduleAt(id, priority, time.Now().Add(delay), job)
}

func (s *Scheduler) run() {
	timer := time.NewTimer(time.Hour) // Will be reset as needed
	defer timer.Stop()

	for {
		s.mu.Lock()
		var nextTask *ScheduledTask
		var waitDuration time.Duration

		if len(s.queue) > 0 {
			nextTask = s.queue[0]
			waitDuration = time.Until(nextTask.ScheduleAt)
		} else {
			waitDuration = time.Hour
		}
		s.mu.Unlock()

		if waitDuration <= 0 && nextTask != nil {
			s.executeNext()
			continue
		}

		timer.Reset(waitDuration)

		select {
		case <-s.ctx.Done():
			return
		case <-s.notify:
			// New task added, re-evaluate
			timer.Stop()
		case <-timer.C:
			s.executeNext()
		}
	}
}

func (s *Scheduler) executeNext() {
	s.mu.Lock()
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}

	st := heap.Pop(&s.queue).(*ScheduledTask)
	s.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(task.Event{
			Type:      task.EventQueued,
			TaskID:    st.ID,
			Timestamp: time.Now(),
		})
	}

	// Submit to worker pool
	_ = s.pool.Submit(s.ctx, st.Job)
}

// Pending returns the number of pending tasks.
func (s *Scheduler) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// Cancel removes a task from the schedule.
func (s *Scheduler) Cancel(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, st := range s.queue {
		if st.ID == taskID {
			heap.Remove(&s.queue, i)
			return true
		}
	}
	return false
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.cancel()
}

// Stats returns scheduler statistics.
func (s *Scheduler) Stats() SchedulerStats {
	s.mu.Lock()
	pending := len(s.queue)
	s.mu.Unlock()

	poolStats := s.pool.Stats()

	return SchedulerStats{
		Pending:   pending,
		Active:    poolStats.Active,
		Workers:   poolStats.Workers,
		Completed: poolStats.Completed,
		Failed:    poolStats.Failed,
	}
}

// SchedulerStats contains scheduler statistics.
type SchedulerStats struct {
	Pending   int   `json:"pending"`
	Active    int   `json:"active"`
	Workers   int   `json:"workers"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
}
