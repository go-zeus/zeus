# 17-job-cron · cron 表达式任务调度

与 [16-job](../16-job/)（固定间隔）互补，演示基于 `robfig/cron/v3` 的 cron 表达式调度。走 `plugins/job/cron`（独立 module，带第三方依赖）。

## 演示场景

- 5 字段标准 cron：`*/5 * * * *`（每 5 分钟）、`0 9 * * 1-5`（工作日 9 点）
- `@every` 简写：`@every 1s` / `@every 2s`
- 通过 URL scheme `cron://` 构造调度器

## 启动与测试

```bash
cd examples/17-job-cron
go run .
```

预期输出（按 cron 表达式触发）：

```
[INFO] cron scheduler started with 3 job(s)
[INFO] tick-every-1s
[INFO] tick-every-2s
```

## 核心代码

```go
import _ "github.com/go-zeus/zeus/plugins/job/cron"   // 注册 cron:// scheme

// 通过 URL scheme 构造（也可直接 cron.New()）
scheduler, _ := job.NewSchedulerFromURL("cron://?seconds=false&loc=UTC")

tick := job.Spec{
    Name:     "tick-1s",
    Schedule: "@every 1s",   // cron 表达式（替代 interval 的 Every 字段）
    Handler:  func(ctx context.Context) error { ... },
}
```

## 与 16-job 的对照

| 维度 | 16-job（interval） | 17-job-cron |
|---|---|---|
| 字段 | `Every: 1*time.Second` | `Schedule: "@every 1s"` |
| 实现 | `job/interval`（零依赖） | `plugins/job/cron`（robfig/cron） |
| 适用 | 心跳/上报/清理（固定间隔） | 工作日 9 点 / 复杂时间规则 |

## 衔接

- 固定间隔调度 → [16-job](../16-job/)
- 声明式组件装配 → [06-autoapp-full](../06-autoapp-full/)
