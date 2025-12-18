# Asynctask (Go)

一个**可上生产**的极简 Go 异步任务小框架：只实现核心能力——**取消、等待结果、进度通道**。

## 核心设计（为什么适合生产）

- **取消**：完全基于 `context.Context`，不会自造一套取消机制。
- **不阻塞进度**：进度通道满时会**丢弃**事件，避免慢消费者拖死任务线程。
- **panic 安全**：任务函数 panic 会被 recover 并转成 `*asynctask.PanicError`，避免后台 goroutine 直接把进程打崩。

## 公共 API（只有核心）

- `NewTask[Params, Progress, Result](params, fn, ...opts) *Task`
- `(*Task).Start(ctx) *Handle`
- `(*Handle).Progress() <-chan Progress`
- `(*Handle).Done() <-chan struct{}`
- `(*Handle).Await(ctx) (Result, error)`
- `(*Handle).Cancel()`
- `WithProgressBuffer(n int)`：设置进度通道缓冲（默认 16）

## 示例

### 1) 基础用法

```go
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
```

### 2) 进度汇报

```go
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
```

## 运行

```bash
go run ./examples/basic
go run ./examples/progress
```

## 测试

```bash
go test ./...
```
