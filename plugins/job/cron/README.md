# cron 表达式调度

`job.Scheduler` 的 [robfig/cron](https://github.com/robfig/cron) 实现。用标准 cron 表达式描述复杂调度规则（如「工作日 9 点跑报表」、「每小时第 30 分清理缓存」），与内置 `job/interval`（固定间隔）形成互补。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/job/cron
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
import (
    "context"
    "time"

    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/job"
    cronimpl "github.com/go-zeus/zeus/plugins/job/cron"
)

app := components.NewApp(
    components.NewJobComponent(cronimpl.New(
        cronimpl.WithLocation(time.Local),
    )),
    components.NewJobRegistration(job.Spec{
        Name:     "daily-report",
        Schedule: "0 9 * * 1-5", // 工作日 9 点
        Handler: func(ctx context.Context) error {
            return generateReport(ctx)
        },
        Timeout: 5 * time.Minute,
    }),
    components.NewJobRegistration(job.Spec{
        Name:     "cache-cleanup",
        Schedule: "*/30 * * * *", // 每 30 分钟
        Handler:  cleanupCache,
    }),
)
app.Run()
```

URL scheme 切换（无需改代码）：

```go
import _ "github.com/go-zeus/zeus/plugins/job/cron" // 注册 cron://

s, _ := job.NewSchedulerFromURL("cron://?seconds=true&loc=Asia/Shanghai")
```

`interval://` 与 `cron://` 读 `Spec` 的不同字段（`Every` vs `Schedule`），可在同一 Spec 列表混用。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithLocation(loc)` | `*time.Location` | `time.Local` | cron 表达式默认时区；单个 Spec 可用 `"CRON_TZ=Asia/Shanghai 0 9 * * *"` 前缀覆盖 |
| `WithSeconds()` | — | 关闭 | 启用 6 字段表达式（含秒）：`"*/30 * * * * *"` |
| `WithErrorHandler(h)` | `job.ErrorHandler` | `log.Error` | Job 执行失败的回调，可对接告警 / 重试系统 |
| `WithoutRecovery()` | — | 关闭 | 关闭 panic 恢复（默认开启），不推荐生产使用 |

默认安全配置（`cron.Recover` + `cron.SkipIfStillRunning`）：单 Job panic 不会影响其他 Job，慢任务会被跳过避免重叠堆积。

URL scheme 支持的 query 参数：

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `seconds` | bool | `false` | 启用 6 字段表达式（含秒） |
| `loc` | string（IANA） | `Local` | 默认时区，如 `Asia/Shanghai` |

不识别的 query 参数静默忽略，前向兼容。

## 依赖

- `github.com/robfig/cron/v3`（cron 表达式解析与调度）

## 集成

- 与 `JobComponent` 配合：声明式注册 + 自动启停，关闭时等待运行中 Job 完成（context 取消路径视为正常退出，不报错）
- 与 `job.Spec.Validate` 配合：`Register` 阶段提前校验 cron 表达式语法，避免 `Start` 时才发现错误
- 与 cluster 治理联动：`Handler` 的 ctx 在 `Stop` 时被取消，业务可读 ctx 内 cluster 标记做差异化执行
- 与内置 `job/interval` 互补：固定间隔走 interval（零依赖），cron 规则走本插件
- 示例参考 `examples/job-cron/`
