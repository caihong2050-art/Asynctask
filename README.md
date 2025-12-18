# Asynctask (Go)

> 一个轻量、可取消、带进度汇报的 Go 异步任务框架。

## 特性

- **泛型任务定义**：`Task[Params, Progress, Result]`。
- **可取消**：基于 `context.Context`，支持 `Handle.Cancel()`。
- **进度汇报**：通过 `ProgressReporter.Report()` 写入进度通道。
- **两种执行方式**：
  - 直接 `Start()`：每个任务一个 goroutine。
  - 通过 `Runner`：worker-pool 执行（可选）。
- **可控的进度行为**：缓冲大小、满了是阻塞还是丢弃。
- **panic 转 error（可选）**：避免把 panic 直接炸到调用栈之外。

## 快速开始

### 基础用法

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

### 进度汇报

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
    asynctask.WithProgressMode(asynctask.ProgressDropIfFull),
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

## 公共 API 概览

### `Task[Params, Progress, Result]`

- **构造**：`NewTask(params, fn, ...opts)`
- **启动**：
  - `Start(ctx)`：直接起 goroutine
  - `Submit(ctx, runner)`：丢进 `Runner` worker-pool

### `Handle[Progress, Result]`

- **等待结束**：`Await(ctx)`
- **取消**：`Cancel()`
- **完成信号**：`Done() <-chan struct{}`
- **进度通道**：`Progress() <-chan Progress`

### `Runner`

- **创建**：`NewRunner(workerCount, queueSize)`
- **关闭并等待**：`Close()`

## 运行示例

```bash
go run ./examples/basic
go run ./examples/progress
```

## 测试

```bash
go test ./...
```
