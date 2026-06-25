// Example: job-cron
//
// 演示基于 robfig/cron/v3 的 cron 表达式任务调度（与内置 interval 互补）：
//   - 用 5 字段标准 cron 表达式："*/5 * * * *"（每 5 分钟）、"0 9 * * 1-5"（工作日 9 点）
//   - 用 @every 简写：@every 1s / @every 2s
//   - 用 URL scheme "cron://" 通过 resolver 构造调度器
//   - 用 components 自动装配：注册 + 启动 + 优雅关闭 全部自动化
//
// 与 examples/job 的对照：
//   - examples/job: 用 interval + Every 字段（固定间隔）
//   - examples/job-cron: 用 cron + Schedule 字段（cron 表达式）
//
// 启动：
//
//	go run .
//
// 预期输出：
//
//	[INFO] cron job demo starting；Ctrl+C to stop
//	[INFO] cron scheduler started with 3 job(s)
//	[INFO] tick-every-1s
//	[INFO] tick-every-2s
//	... (按 cron 表达式触发)
//
// Ctrl+C 退出（优雅关闭，所有 Job goroutine 退出）。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/job"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"

	// 副作用 import：注册 cron:// scheme 到 job.RegisterResolver
	cronplugin "github.com/go-zeus/zeus/plugins/job/cron"
)

func main() {
	// 通过 URL scheme 构造调度器：cron://?seconds=true 启用 6 字段（含秒）表达式
	// 也可直接 cronplugin.New(cronplugin.WithSeconds()) 构造
	scheduler, err := job.NewSchedulerFromURL("cron://")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create scheduler: %v\n", err)
		os.Exit(1)
	}

	// 1) tick-1s：每 1s 触发（用 @every 简写，避免 cron 最小粒度 1 分钟）
	tick1s := job.Spec{
		Name:     "tick-1s",
		Schedule: "@every 1s",
		Handler: func(ctx context.Context) error {
			log.Info("tick-every-1s")
			return nil
		},
	}

	// 2) tick-2s：每 2s 触发
	tick2s := job.Spec{
		Name:     "tick-2s",
		Schedule: "@every 2s",
		Handler: func(ctx context.Context) error {
			log.Info("tick-every-2s")
			return nil
		},
	}

	// 3) simulated-failure：每 3s 故意失败，演示 ErrorHandler 路径
	failing := job.Spec{
		Name:     "simulated-failure",
		Schedule: "@every 3s",
		Handler: func(ctx context.Context) error {
			return fmt.Errorf("simulated failure for testing ErrorHandler")
		},
		Timeout: 500 * time.Millisecond,
	}

	// 装配：JobComponent 持有 Scheduler，每个 NewJobRegistration 声明一个 Job
	// App 启动时自动注册 → Start；停止时自动 Stop
	//
	// 注意：本例直接使用 cronplugin.New() 以便注入自定义 ErrorHandler；
	// URL scheme 路径在 main 顶部已展示（用于切换实现包）。
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewJobComponent(
			cronplugin.New(cronplugin.WithErrorHandler(func(name string, err error) {
				log.Error("ALERT: cron job %s failed: %v", name, err)
			})),
		),
		components.NewJobRegistration(tick1s),
		components.NewJobRegistration(tick2s),
		components.NewJobRegistration(failing),
	)

	_ = scheduler // URL scheme 路径已展示，此处不重复使用

	log.Info("cron job demo starting；Ctrl+C to stop")
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}
