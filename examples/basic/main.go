package main

import (
	"context"
	"fmt"
	"time"

	"asynctask/asynctask"
)

func main() {
	t := asynctask.NewTask[string, struct{}, string](
		"Hello, World!",
		func(ctx context.Context, p string, _ asynctask.ProgressReporter[struct{}]) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return "Result: " + p, nil
			}
		},
	)

	h := t.Start(context.Background())
	res, err := h.Await(context.Background())
	fmt.Println(res, err)
}
