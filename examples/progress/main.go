package main

import (
	"context"
	"fmt"
	"time"

	"asynctask/asynctask"
)

func main() {
	t := asynctask.NewTask[int, int, string](
		100,
		func(ctx context.Context, total int, progress asynctask.ProgressReporter[int]) (string, error) {
			for i := 0; i <= total; i += 10 {
				if !progress.Report(i) {
					return "", ctx.Err()
				}
				time.Sleep(20 * time.Millisecond)
			}
			return "Task Completed!", nil
		},
		asynctask.WithProgressBuffer(8),
	)

	h := t.Start(context.Background())

	go func() {
		for p := range h.Progress() {
			fmt.Printf("Progress: %d%%\n", p)
		}
	}()

	res, err := h.Await(context.Background())
	fmt.Println(res, err)
}
