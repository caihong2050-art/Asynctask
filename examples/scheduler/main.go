package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"asynctask/domain/executor"
	"asynctask/domain/task"
)

func main() {
	fmt.Println("=== Scheduler & Priority Example ===")
	fmt.Println()

	// Create executor
	exec := executor.New(executor.WithMaxWorkers(2))
	defer exec.Stop()

	// Track execution order
	var mu sync.Mutex
	executionOrder := []string{}

	// Subscribe to start events
	exec.EventBus().Subscribe(task.EventStarted, func(e task.Event) {
		id := e.TaskID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Printf("[Started] Task %s\n", id)
	})

	exec.EventBus().Subscribe(task.EventCompleted, func(e task.Event) {
		id := e.TaskID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Printf("[Completed] Task %s\n", id)
	})

	fmt.Println("1. Scheduling tasks with different priorities:")

	// Schedule tasks with different priorities
	// They should execute in priority order when workers become available

	scheduleTask := func(name string, priority task.Priority, delay time.Duration) {
		taskID, err := executor.Schedule(
			exec,
			context.Background(),
			time.Now().Add(delay),
			priority,
			name,
			func(ctx context.Context, n string, _ task.ProgressReporter[struct{}]) (string, error) {
				mu.Lock()
				executionOrder = append(executionOrder, n)
				mu.Unlock()
				
				fmt.Printf("   Executing: %s (priority: %s)\n", n, priority)
				time.Sleep(100 * time.Millisecond)
				return n + " done", nil
			},
			task.WithName(name),
		)
		if err != nil {
			fmt.Printf("Failed to schedule %s: %v\n", name, err)
			return
		}
		fmt.Printf("   Scheduled: %s (ID: %s, priority: %s)\n", name, taskID[:8], priority)
	}

	// Schedule tasks (lower priority first, but higher priority should execute first)
	scheduleTask("low-priority-task", task.PriorityLow, 0)
	scheduleTask("normal-priority-task", task.PriorityNormal, 0)
	scheduleTask("high-priority-task", task.PriorityHigh, 0)
	scheduleTask("critical-task", task.PriorityCritical, 0)

	// Wait for all tasks to complete
	time.Sleep(800 * time.Millisecond)
	fmt.Println()

	fmt.Println("Execution order:", executionOrder)
	fmt.Println()

	// Example 2: Delayed task scheduling
	fmt.Println("2. Delayed task scheduling:")
	executionOrder = []string{}

	taskID1, _ := executor.ScheduleAfter(
		exec,
		context.Background(),
		200*time.Millisecond,
		task.PriorityNormal,
		"delayed-200ms",
		func(ctx context.Context, n string, _ task.ProgressReporter[struct{}]) (string, error) {
			mu.Lock()
			executionOrder = append(executionOrder, n)
			mu.Unlock()
			fmt.Printf("   Executed: %s\n", n)
			return n, nil
		},
	)
	fmt.Printf("   Scheduled delayed task: %s\n", taskID1[:8])

	taskID2, _ := executor.ScheduleAfter(
		exec,
		context.Background(),
		100*time.Millisecond,
		task.PriorityNormal,
		"delayed-100ms",
		func(ctx context.Context, n string, _ task.ProgressReporter[struct{}]) (string, error) {
			mu.Lock()
			executionOrder = append(executionOrder, n)
			mu.Unlock()
			fmt.Printf("   Executed: %s\n", n)
			return n, nil
		},
	)
	fmt.Printf("   Scheduled delayed task: %s\n", taskID2[:8])

	// Wait for delayed tasks
	time.Sleep(500 * time.Millisecond)
	fmt.Println()

	fmt.Println("Execution order (by delay):", executionOrder)
	fmt.Println()

	// Print stats
	stats := exec.Stats()
	fmt.Println("Final Stats:")
	fmt.Printf("   Completed: %d, Active: %d, Pending: %d\n", stats.Completed, stats.Active, stats.Pending)
}
