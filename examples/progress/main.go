package main

import (
	"context"
	"fmt"
	"time"

	"asynctask/domain/executor"
	"asynctask/domain/task"
)

// ProgressInfo represents progress information
type ProgressInfo struct {
	Current int
	Total   int
	Message string
}

func main() {
	fmt.Println("=== Progress Reporting Example ===")
	fmt.Println()

	// Create executor
	exec := executor.New(executor.WithMaxWorkers(4))
	defer exec.Stop()

	// Task with progress reporting
	handle, err := executor.Submit(
		exec,
		context.Background(),
		100, // Total steps
		func(ctx context.Context, total int, progress task.ProgressReporter[ProgressInfo]) (string, error) {
			for i := 0; i <= total; i += 10 {
				// Check for cancellation
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				default:
				}

				// Report progress
				if !progress.Report(ProgressInfo{
					Current: i,
					Total:   total,
					Message: fmt.Sprintf("Processing step %d/%d", i, total),
				}) {
					return "", ctx.Err()
				}

				// Simulate work
				time.Sleep(50 * time.Millisecond)
			}
			return "Task completed successfully!", nil
		},
		task.WithName("progress-task"),
		task.WithProgressBuffer(16),
	)
	if err != nil {
		fmt.Printf("Failed to submit task: %v\n", err)
		return
	}

	// Consume progress in a separate goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range handle.Progress() {
			percentage := float64(p.Current) / float64(p.Total) * 100
			fmt.Printf("\r[%-50s] %.0f%% - %s",
				progressBar(percentage, 50),
				percentage,
				p.Message,
			)
		}
		fmt.Println()
	}()

	// Wait for result
	result, err := handle.Await(context.Background())
	<-done // Wait for progress display to finish

	fmt.Println()
	if err != nil {
		fmt.Printf("Task failed: %v\n", err)
	} else {
		fmt.Printf("Result: %s\n", result)
	}

	fmt.Printf("Duration: %s\n", handle.Duration())
	fmt.Printf("Final State: %s\n", handle.State())
}

// progressBar generates a text progress bar
func progressBar(percentage float64, width int) string {
	filled := int(percentage / 100 * float64(width))
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}
