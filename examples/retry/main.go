package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"asynctask/domain/executor"
	"asynctask/domain/task"
	"asynctask/pkg/retry"
)

func main() {
	fmt.Println("=== Retry Strategy Example ===")
	fmt.Println()

	// Create executor
	exec := executor.New(executor.WithMaxWorkers(4))
	defer exec.Stop()

	// Subscribe to retry events
	exec.EventBus().Subscribe(task.EventRetry, func(e task.Event) {
		id := e.TaskID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Printf("   [Retry] Task %s: retrying...\n", id)
	})

	// Counter to track attempts
	attempts := 0

	// Task that fails a few times before succeeding
	fmt.Println("1. Task with exponential backoff retry:")
	strategy := &retry.ExponentialStrategy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Attempts:     5,
		Jitter:       true,
	}

	handle, err := executor.SubmitWithRetry(
		exec,
		context.Background(),
		3, // Fail 3 times before success
		func(ctx context.Context, failCount int, _ task.ProgressReporter[struct{}]) (string, error) {
			attempts++
			fmt.Printf("   Attempt #%d\n", attempts)
			if attempts <= failCount {
				return "", errors.New("simulated failure")
			}
			return "Success after retries!", nil
		},
		strategy,
		task.WithName("retry-task"),
	)
	if err != nil {
		fmt.Printf("   Failed to submit task: %v\n", err)
		return
	}

	result, err := handle.Await(context.Background())
	fmt.Printf("   Result: %s, Error: %v\n", result, err)
	fmt.Printf("   Total attempts: %d\n", attempts)
	fmt.Println()

	// Reset counter
	attempts = 0

	// Task that exhausts all retries
	fmt.Println("2. Task that exhausts retries:")
	strategy2 := &retry.FixedStrategy{
		Delay:    50 * time.Millisecond,
		Attempts: 3,
	}

	handle2, err := executor.SubmitWithRetry(
		exec,
		context.Background(),
		struct{}{},
		func(ctx context.Context, _ struct{}, _ task.ProgressReporter[struct{}]) (string, error) {
			attempts++
			fmt.Printf("   Attempt #%d - failing...\n", attempts)
			return "", errors.New("persistent failure")
		},
		strategy2,
		task.WithName("always-fail-task"),
	)
	if err != nil {
		fmt.Printf("   Failed to submit task: %v\n", err)
		return
	}

	result2, err := handle2.Await(context.Background())
	fmt.Printf("   Result: %s, Error: %v\n", result2, err)
	fmt.Printf("   Total attempts: %d\n", attempts)
	fmt.Println()

	// Print stats
	stats := exec.Stats()
	fmt.Println("Executor Stats:")
	fmt.Printf("   Completed: %d, Failed: %d\n", stats.Completed, stats.Failed)
}
