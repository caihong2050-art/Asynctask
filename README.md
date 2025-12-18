# Asynctask (Go)

一个**生产级**的 Go 异步任务框架，采用 **领域驱动设计 (DDD)** 架构，提供完整的任务生命周期管理、优先级调度、重试机制、进度报告和可观测性支持。

## ✨ 特性

- **领域驱动设计 (DDD)** - 清晰的分层架构，高度可扩展
- **泛型支持** - 类型安全的任务参数、进度和结果
- **完整的生命周期管理** - 任务状态机、事件驱动
- **优先级调度** - 支持 Low/Normal/High/Critical 四级优先级
- **智能重试** - 固定/指数/线性退避策略
- **Worker Pool** - 高效的并发任务执行
- **进度报告** - 非阻塞的进度通道
- **钩子系统** - 灵活的生命周期钩子
- **指标收集** - 内置可观测性支持
- **Panic 安全** - 自动捕获和恢复 panic
- **超时控制** - 任务级别的超时设置
- **取消支持** - 完全基于 `context.Context`

## 📁 项目结构

```
asynctask/
├── domain/                 # 领域层 - 核心业务逻辑
│   ├── task/              # 任务聚合根
│   │   ├── task.go        # 任务实体与 Handle
│   │   ├── state.go       # 状态机
│   │   ├── events.go      # 领域事件
│   │   └── config.go      # 配置选项
│   ├── executor/          # 执行器（统一入口）
│   │   └── executor.go
│   └── errors/            # 领域错误
│       └── errors.go
├── application/           # 应用层 - 用例编排
│   ├── service/           # 应用服务
│   │   └── task_service.go
│   └── dto/               # 数据传输对象
│       └── task_dto.go
├── infrastructure/        # 基础设施层
│   ├── scheduler/         # 优先级调度器
│   │   └── scheduler.go
│   ├── pool/              # Worker Pool
│   │   └── worker_pool.go
│   └── metrics/           # 指标收集
│       └── metrics.go
├── pkg/                   # 公共工具包
│   ├── retry/             # 重试策略
│   │   └── retry.go
│   └── hooks/             # 钩子系统
│       └── hooks.go
└── examples/              # 示例代码
    ├── basic/             # 基础用法
    ├── progress/          # 进度报告
    ├── retry/             # 重试策略
    ├── scheduler/         # 优先级调度
    └── advanced/          # 高级特性
```

## 🚀 快速开始

### 安装

```bash
go get asynctask
```

### 基础用法

```go
package main

import (
    "context"
    "fmt"
    "time"

    "asynctask/domain/executor"
    "asynctask/domain/task"
)

func main() {
    // 创建执行器
    exec := executor.New(
        executor.WithMaxWorkers(4),
        executor.WithMetrics(true),
    )
    defer exec.Stop()

    // 提交任务
    handle, err := executor.Submit(
        exec,
        context.Background(),
        "Hello, World!",
        func(ctx context.Context, msg string, _ task.ProgressReporter[struct{}]) (string, error) {
            time.Sleep(100 * time.Millisecond)
            return "Processed: " + msg, nil
        },
        task.WithName("greeting-task"),
    )
    if err != nil {
        panic(err)
    }

    // 等待结果
    result, err := handle.Await(context.Background())
    fmt.Printf("Result: %s, Error: %v\n", result, err)
}
```

### 进度报告

```go
handle, _ := executor.Submit(
    exec,
    context.Background(),
    100,
    func(ctx context.Context, total int, progress task.ProgressReporter[int]) (string, error) {
        for i := 0; i <= total; i += 10 {
            if !progress.Report(i) {
                return "", ctx.Err()
            }
            time.Sleep(50 * time.Millisecond)
        }
        return "Done!", nil
    },
    task.WithProgressBuffer(16),
)

// 消费进度
go func() {
    for p := range handle.Progress() {
        fmt.Printf("Progress: %d%%\n", p)
    }
}()

result, _ := handle.Await(context.Background())
```

### 重试机制

```go
import "asynctask/pkg/retry"

strategy := &retry.ExponentialStrategy{
    InitialDelay: 100 * time.Millisecond,
    MaxDelay:     5 * time.Second,
    Multiplier:   2.0,
    Attempts:     5,
    Jitter:       true,
}

handle, _ := executor.SubmitWithRetry(
    exec,
    context.Background(),
    params,
    myFunc,
    strategy,
)
```

### 优先级调度

```go
// 高优先级任务
taskID, _ := executor.Schedule(
    exec,
    context.Background(),
    time.Now().Add(5*time.Second),
    task.PriorityHigh,
    params,
    myFunc,
)

// 延迟执行
executor.ScheduleAfter(exec, ctx, 10*time.Second, task.PriorityNormal, params, fn)
```

### 事件驱动

```go
// 订阅事件
exec.EventBus().Subscribe(task.EventCompleted, func(e task.Event) {
    fmt.Printf("Task %s completed\n", e.TaskID)
})

exec.EventBus().Subscribe(task.EventFailed, func(e task.Event) {
    fmt.Printf("Task %s failed\n", e.TaskID)
})
```

### 钩子系统

```go
import "asynctask/pkg/hooks"

exec.Hooks().Register(hooks.PhaseBeforeSubmit, func(ctx context.Context, hctx *hooks.HookContext) error {
    fmt.Printf("Task %s is about to be submitted\n", hctx.TaskID)
    return nil
}, hooks.WithName("logging-hook"))
```

## 📊 任务状态

任务支持以下状态转换：

```
┌─────────┐    ┌────────┐    ┌─────────┐    ┌───────────┐
│ Pending │───▶│ Queued │───▶│ Running │───▶│ Completed │
└─────────┘    └────────┘    └─────────┘    └───────────┘
     │              │             │
     │              │             ├───▶ Failed
     │              │             │
     │              │             ├───▶ Cancelled
     │              │             │
     └──────────────┴─────────────┴───▶ Timeout
```

## 🔧 配置选项

### Executor 配置

```go
exec := executor.New(
    executor.WithMaxWorkers(10),        // 最大工作协程数
    executor.WithQueueSize(100),        // 任务队列大小
    executor.WithDefaultTimeout(30*time.Second),  // 默认超时
    executor.WithRetryStrategy(retry.DefaultStrategy()), // 默认重试策略
    executor.WithMetrics(true),         // 启用指标收集
)
```

### Task 配置

```go
task.NewTask(params, fn,
    task.WithID("custom-id"),           // 自定义 ID
    task.WithName("my-task"),           // 任务名称
    task.WithPriority(task.PriorityHigh), // 优先级
    task.WithTimeout(10*time.Second),   // 超时时间
    task.WithProgressBuffer(32),        // 进度缓冲区大小
    task.WithRetry(3, 100*time.Millisecond), // 重试配置
    task.WithMetadata("key", "value"),  // 自定义元数据
)
```

## 📈 指标与监控

```go
// 获取指标快照
snapshot := exec.Metrics().Snapshot()

fmt.Printf("Tasks Created: %d\n", snapshot.TasksCreated)
fmt.Printf("Tasks Completed: %d\n", snapshot.TasksCompleted)
fmt.Printf("Tasks Failed: %d\n", snapshot.TasksFailed)
fmt.Printf("Success Rate: %.2f%%\n", snapshot.SuccessRate()*100)
fmt.Printf("P50 Duration: %s\n", snapshot.DurationP50)
fmt.Printf("P99 Duration: %s\n", snapshot.DurationP99)
```

## 🔄 重试策略

框架提供三种内置重试策略：

1. **FixedStrategy** - 固定间隔重试
2. **ExponentialStrategy** - 指数退避重试（推荐）
3. **LinearStrategy** - 线性增长重试

```go
// 指数退避（推荐）
strategy := &retry.ExponentialStrategy{
    InitialDelay: 100 * time.Millisecond,
    MaxDelay:     10 * time.Second,
    Multiplier:   2.0,
    Attempts:     5,
    Jitter:       true, // 添加随机抖动，避免惊群效应
}
```

## 🧪 运行示例

```bash
# 基础用法
go run ./examples/basic

# 进度报告
go run ./examples/progress

# 重试机制
go run ./examples/retry

# 优先级调度
go run ./examples/scheduler

# 高级特性
go run ./examples/advanced
```

## 🧪 运行测试

```bash
go test ./... -v
```

## 📄 License

MIT License
