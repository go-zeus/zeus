// Example: job
//
// 演示声明式任务调度（Job）：
//   - 定义 3 个周期性 Job：heartbeat / metrics-reporter / cleanup
//   - 用 components 自动装配：注册 + 启动 + 优雅关闭 全部自动化
//   - 每个 Job 触发时打印 log，验证周期性执行
//
// 启动：
//
//	go run .
//
// 预期输出（每 1s 左右一组）：
//
//	[INFO] job scheduler started with 3 job(s)
//	[INFO] heartbeat tick
//	[INFO] metrics-reporter publishing count=1
//	[INFO] cleanup done
//	... (按各自间隔重复)
//
// Ctrl+C 退出（优雅关闭，所有 Job goroutine 退出）。
package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/job"
	"github.com/go-zeus/zeus/job/interval"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"
)

func main() {
	var counter int64

	// 1) heartbeat：每 1s 上报一次"心跳"（高频任务典型用例）
	heartbeat := job.Spec{
		Name:  "heartbeat",
		Every: 1 * time.Second,
		Handler: func(ctx context.Context) error {
			// 注意：包级 log.Info 不接受 ctx（保持简洁）
			// 需 ctx-aware 日志时用 log.Default().Log(ctx, log.LevelInfo, msg)
			log.Info("heartbeat tick")
			return nil
		},
	}

	// 2) metrics-reporter：每 2s 把累积计数"上报"（中频聚合任务典型用例）
	metricsReporter := job.Spec{
		Name:  "metrics-reporter",
		Every: 2 * time.Second,
		Handler: func(ctx context.Context) error {
			n := atomic.AddInt64(&counter, 1)
			log.Info("metrics-reporter publishing count=%d", n)
			return nil
		},
	}

	// 3) cleanup：每 3s 执行一次清理（低频维护任务典型用例）
	cleanup := job.Spec{
		Name:  "cleanup",
		Every: 3 * time.Second,
		// 演示 Timeout：模拟一次执行最多 100ms（实际业务可能更长）
		Timeout: 100 * time.Millisecond,
		Handler: func(ctx context.Context) error {
			log.Info("cleanup done")
			return nil
		},
	}

	// 4) 模拟一个会失败的 Job（验证 ErrorHandler 路径）
	//    用户可通过 WithErrorHandler 注入告警/重试逻辑
	failing := job.Spec{
		Name:  "failing-job",
		Every: 5 * time.Second,
		Handler: func(ctx context.Context) error {
			return fmt.Errorf("simulated failure for testing ErrorHandler")
		},
	}

	// 装配：JobComponent 持有 Scheduler，每个 NewJobRegistration 声明一个 Job
	// App 启动时自动注册 → Start；停止时自动 Stop
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewJobComponent(
			interval.New(interval.WithErrorHandler(func(name string, err error) {
				// 默认错误处理：log.Error；这里演示告警钩子
				log.Error("ALERT: job %s failed: %v", name, err)
			})),
		),
		components.NewJobRegistration(heartbeat),
		components.NewJobRegistration(metricsReporter),
		components.NewJobRegistration(cleanup),
		components.NewJobRegistration(failing),
	)

	log.Info("job demo starting；Ctrl+C to stop")
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}
