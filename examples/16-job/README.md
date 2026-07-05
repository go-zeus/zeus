# 16-job · 周期任务调度（interval）

演示 `job` 包的声明式周期任务调度：定义多个固定间隔 Job，components 自动装配启停。基于内置 `job/interval`（零依赖，`time.Ticker`）。

## 演示场景

3 个周期性 Job：
- `heartbeat`：心跳上报
- `metrics-reporter`：指标上报
- `cleanup`：定期清理

每个 Job 独立 goroutine + 独立 Ticker，单 Job panic 不影响其他。

## 启动与测试

```bash
cd examples/16-job
go run .
```

预期输出（按各自间隔重复）：

```
[INFO] job scheduler started with 3 job(s)
[INFO] heartbeat tick
[INFO] metrics-reporter publishing count=1
[INFO] cleanup done
```

Ctrl+C 优雅关闭，所有 Job goroutine 退出。

## 核心代码

```go
scheduler := interval.New()

heartbeat := job.Spec{
    Name:  "heartbeat",
    Every: 1 * time.Second,
    Handler: func(ctx context.Context) error {
        return reportHeartbeat(ctx)
    },
    Timeout: 5 * time.Second,   // 单次执行超时
}

app := components.NewApp(
    components.NewJobComponent(scheduler),
    components.NewJobRegistration(heartbeat),
)
app.Run()
```

## 衔接

- cron 表达式调度 → [17-job-cron](../17-job-cron/)
- 消息队列（事件驱动） → [15-mq](../15-mq/)
