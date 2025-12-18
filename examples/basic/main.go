package main

import (
	"context"
	"fmt"
	"time"

	"asynctask/domain/executor"
	"asynctask/domain/task"
)

func main() {
	fmt.Println("=== Basic Usage Example ===")
	fmt.Println()

	// Create executor with default settings
	exec := executor.New(
		executor.WithMaxWorkers(4),
		executor.WithMetrics(true),
	)
	defer exec.Stop()

	// Example 1: Simple task execution
	fmt.Println("1. Simple task execution:")
	handle, err := executor.Submit(
		exec,
		context.Background(),
		"Hello, World!",
		func(ctx context.Context, msg string, _ task.ProgressReporter[struct{}]) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return "Processed: " + msg, nil
			}
		},
		task.WithName("greeting-task"),
	)
	if err != nil {
		fmt.Printf("   Failed to submit task: %v\n", err)
		return
	}

	result, err := handle.Await(context.Background())
	fmt.Printf("   Result: %s, Error: %v\n", result, err)
	fmt.Printf("   Duration: %s\n", handle.Duration())
	fmt.Println()

	// Example 2: Task with timeout
	fmt.Println("2. Task with timeout:")
	handle2, err := executor.Submit(
		exec,
		context.Background(),
		struct{}{},
		func(ctx context.Context, _ struct{}, _ task.ProgressReporter[struct{}]) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
				return "completed", nil
			}
		},
		task.WithName("timeout-task"),
		task.WithTimeout(100*time.Millisecond),
	)
	if err != nil {
		fmt.Printf("   Failed to submit task: %v\n", err)
		return
	}

	result2, err := handle2.Await(context.Background())
	fmt.Printf("   Result: %s, Error: %v\n", result2, err)
	fmt.Printf("   State: %s\n", handle2.State())
	fmt.Println()

	// Example 3: Task cancellation
	fmt.Println("3. Task cancellation:")
	handle3, _ := executor.Submit(
		exec,
		context.Background(),
		struct{}{},
		func(ctx context.Context, _ struct{}, _ task.ProgressReporter[struct{}]) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "completed", nil
			}
		},
		task.WithName("long-task"),
	)

	// Cancel after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		handle3.Cancel()
	}()

	result3, err := handle3.Await(context.Background())
	fmt.Printf("   Result: %s, Error: %v\n", result3, err)
	fmt.Printf("   State: %s\n", handle3.State())
	fmt.Println()

	// Print executor stats
	stats := exec.Stats()
	fmt.Println("Executor Stats:")
	fmt.Printf("   Workers: %d, Active: %d\n", stats.Workers, stats.Active)
	fmt.Printf("   Completed: %d, Failed: %d\n", stats.Completed, stats.Failed)
	fmt.Printf("   Success Rate: %.2f%%\n", stats.SuccessRate*100)
}
