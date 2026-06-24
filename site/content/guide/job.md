---
title: 任务调度
weight: 100
---

`job` 包提供声明式周期性任务调度抽象。

| 概念 | 说明 |
|---|---|
| `Spec` | 任务规格（Name + Schedule/Every + Handler + Timeout） |
| `Scheduler` | 调度器接口（Register/Start/Stop） |
| `interval.Scheduler` | 内置实现：固定间隔，每 Job 独立 goroutine + Ticker |
| `JobComponent` | components 适配器：声明式注册 + 自动启停 |
| `JobRegistration` | 单个 Job 包装为组件 |

## 设计权衡

| 维度 | 选择 | 理由 |
|---|---|---|
| 内置调度器 | `time.Ticker` 固定间隔 | 零依赖、覆盖 80% 用例（心跳/上报/清理） |
| Cron 表达式 | 放 `plugins/job/cron` | cron 解析复杂，且需要 robfig/cron 依赖 |
| 首次执行 | 立即执行（不延迟一个周期） | 心跳类任务不应延迟首次上报 |
| 并发模型 | 每 Job 独立 goroutine | 隔离故障，单 Job panic 不影响其他 |
| 错误处理 | 默认 log.Error，可注入 ErrorHandler | 用户可对接告警/重试系统 |

## 使用方式

```go
import (
    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/job"
    "github.com/go-zeus/zeus/job/interval"
)

heartbeat := job.Spec{
    Name:  "heartbeat",
    Every: 30 * time.Second,
    Handler: func(ctx context.Context) error {
        return reportHeartbeat(ctx)
    },
    Timeout: 5 * time.Second,
}

app := components.NewApp(
    components.NewJobComponent(interval.New()),
    components.NewJobRegistration(heartbeat),
)
app.Run()
```

## URL scheme 切换调度器实现

通过 `job.NewSchedulerFromURL` 用 URL 字符串切换 interval / cron 实现：

```go
import (
    _ "github.com/go-zeus/zeus/job/interval"       // 注册 interval://
    _ "github.com/go-zeus/zeus/plugins/job/cron"   // 注册 cron://（需在 go.mod require 该插件）
)

s, _ := job.NewSchedulerFromURL("cron://?seconds=true&loc=UTC")
```

| Scheme | 实现 |
|---|---|
| `interval://` | `interval.New()`（固定间隔） |
| `cron://` | `cron.New()`（cron 表达式，支持 `seconds=true` / `loc=Asia/Shanghai` query 参数） |

## 与 cluster 治理的协同

`Handler` 的 ctx 在 `Stop` 时被取消，业务可读取 ctx 内的 cluster 标记做集群差异化执行：

```go
job.Spec{
    Name:  "config-reload",
    Every: 1 * time.Minute,
    Handler: func(ctx context.Context) error {
        cluster := routing.FromContext(ctx) // 默认 default
        return reloadClusterConfig(cluster)
    },
}
```

完整示例：
- `examples/job/`：interval 调度器 + 3 个 Job + ErrorHandler 告警钩子
- `examples/job-cron/`：cron 调度器 + URL scheme
