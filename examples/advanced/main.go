package main

import (
	"context"
	"fmt"
	"time"

	"asynctask/domain/executor"
	"asynctask/domain/task"
	"asynctask/pkg/hooks"
)

// shortID returns the first 8 characters of an ID
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func main() {
	fmt.Println("=== Advanced Features Example ===")
	fmt.Println()

	// Create executor
	exec := executor.New(
		executor.WithMaxWorkers(4),
		executor.WithMetrics(true),
	)
	defer exec.Stop()

	// Example 1: Event-driven programming
	fmt.Println("1. Event-driven programming:")

	exec.EventBus().Subscribe(task.EventCreated, func(e task.Event) {
		fmt.Printf("   [EVENT] Task created: %s\n", shortID(e.TaskID))
	})

	exec.EventBus().Subscribe(task.EventStarted, func(e task.Event) {
		fmt.Printf("   [EVENT] Task started: %s\n", shortID(e.TaskID))
	})

	exec.EventBus().Subscribe(task.EventCompleted, func(e task.Event) {
		fmt.Printf("   [EVENT] Task completed: %s\n", shortID(e.TaskID))
	})

	exec.EventBus().Subscribe(task.EventFailed, func(e task.Event) {
		fmt.Printf("   [EVENT] Task failed: %s\n", shortID(e.TaskID))
	})

	handle, _ := executor.Submit(
		exec,
		context.Background(),
		"test",
		func(ctx context.Context, _ string, _ task.ProgressReporter[struct{}]) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "done", nil
		},
	)
	handle.Await(context.Background())
	fmt.Println()

	// Example 2: Hook system
	fmt.Println("2. Hook system:")

	// Register hooks
	exec.Hooks().Register(hooks.PhaseBeforeSubmit, func(ctx context.Context, hctx *hooks.HookContext) error {
		fmt.Printf("   [HOOK] Before submit: %s\n", shortID(hctx.TaskID))
		hctx.Metadata["start_time"] = time.Now()
		return nil
	}, hooks.WithName("timing-start"))

	exec.Hooks().Register(hooks.PhaseAfterSubmit, func(ctx context.Context, hctx *hooks.HookContext) error {
		fmt.Printf("   [HOOK] After submit: %s\n", shortID(hctx.TaskID))
		return nil
	}, hooks.WithName("log-submit"))

	handle2, _ := executor.Submit(
		exec,
		context.Background(),
		"hooked",
		func(ctx context.Context, _ string, _ task.ProgressReporter[struct{}]) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "done", nil
		},
	)
	handle2.Await(context.Background())
	fmt.Println()

	// Example 3: Concurrent task execution
	fmt.Println("3. Concurrent task execution:")

	const numTasks = 5
	handles := make([]*task.Handle[struct{}, int], numTasks)

	// Submit multiple tasks concurrently
	for i := 0; i < numTasks; i++ {
		taskNum := i
		h, _ := executor.Submit(
			exec,
			context.Background(),
			taskNum,
			func(ctx context.Context, n int, _ task.ProgressReporter[struct{}]) (int, error) {
				time.Sleep(100 * time.Millisecond)
				return n * n, nil
			},
			task.WithName(fmt.Sprintf("compute-%d", taskNum)),
		)
		handles[taskNum] = h
	}

	// Wait for all and collect results
	fmt.Print("   Results: ")
	for i, h := range handles {
		result, _ := h.Await(context.Background())
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%d²=%d", i, result)
	}
	fmt.Println()
	fmt.Println()

	// Example 4: Metrics
	fmt.Println("4. Metrics:")
	snapshot := exec.Metrics().Snapshot()
	fmt.Printf("   Tasks Created: %d\n", snapshot.TasksCreated)
	fmt.Printf("   Tasks Completed: %d\n", snapshot.TasksCompleted)
	fmt.Printf("   Tasks Failed: %d\n", snapshot.TasksFailed)
	fmt.Printf("   Tasks Active: %d\n", snapshot.TasksActive)
	fmt.Printf("   Success Rate: %.2f%%\n", snapshot.SuccessRate()*100)
	fmt.Println()

	// Example 5: Task state inspection
	fmt.Println("5. Task state inspection:")
	
	longHandle, _ := executor.Submit(
		exec,
		context.Background(),
		struct{}{},
		func(ctx context.Context, _ struct{}, _ task.ProgressReporter[struct{}]) (string, error) {
			time.Sleep(200 * time.Millisecond)
			return "done", nil
		},
	)

	fmt.Printf("   Initial state: %s\n", longHandle.State())
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("   During execution: %s\n", longHandle.State())
	longHandle.Await(context.Background())
	fmt.Printf("   After completion: %s\n", longHandle.State())
	fmt.Printf("   Duration: %s\n", longHandle.Duration())
	fmt.Println()

	// Final stats
	stats := exec.Stats()
	fmt.Println("Final Executor Stats:")
	fmt.Printf("   Workers: %d\n", stats.Workers)
	fmt.Printf("   Completed: %d\n", stats.Completed)
	fmt.Printf("   Failed: %d\n", stats.Failed)
	fmt.Printf("   Success Rate: %.2f%%\n", stats.SuccessRate*100)
	fmt.Printf("   Uptime: %s\n", stats.Uptime.Round(time.Millisecond))
}
