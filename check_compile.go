package main

import (
	"context"
	"asynctask/asynctask"
)

func main() {
	// Check if type inference works
	t := asynctask.NewTask(
		"test",
		func(ctx context.Context, p string, r asynctask.ProgressReporter[int]) (string, error) {
			return "", nil
		},
		asynctask.WithProgressBuffer(10), // Implicit instantiation?
	)
	_ = t
}
