// Package job 提供声明式任务调度抽象。
//
// 设计动机：
//   - 让"周期性任务"（如数据清理、缓存预热、心跳上报）像 handler 一样声明式注册
//   - 主包零依赖，仅定义接口 + Spec；内置实现用 time.Ticker（固定间隔）
//   - 复杂 cron 表达式（如 "0 0 * * 1-5"）放 plugins/job/cron
//
// 概念分层：
//
//	Job        = 任务单元（Name + Handler + 调度配置）
//	Scheduler  = 调度器（驱动 Job 按时触发，管理并发）
//
// 与 components 的关系：
//
//	components.NewApp(
//	    components.NewJobComponent(job.NewIntervalScheduler()),
//	    components.NewJobRegistration(job.Spec{
//	        Name: "heartbeat", Every: 30*time.Second,
//	        Handler: func(ctx context.Context) error { ... },
//	    }),
//	)
//
// App 启动时自动注册所有 Job 并启动 Scheduler；停止时优雅关闭。
package job

import (
	"context"
	"fmt"
	"time"
)

// Handler 任务执行函数。
//
// 契约：
//   - ctx 在 Scheduler.Stop 时被取消（用于优雅停止）
//   - 返回 error 由 ErrorHandler 处理（默认 log，可选重试/告警）
//   - Handler 应是幂等的：同一 tick 可能被并发调用（取决于 Scheduler 实现）
type Handler func(ctx context.Context) error

// Spec 任务规格（声明式注册单元）。
//
// Schedule 字段语义：
//   - 内置 interval 实现：仅认 Every 字段（固定间隔），Schedule 被忽略
//   - plugins/job/cron 实现：认 Schedule 字段（cron 表达式），Every 被忽略
//
// 设计理由：用单个 Spec 类型同时承载两种调度模型，避免接口分裂；
// 不同 Scheduler 实现读取不同字段，互不冲突。
type Spec struct {
	// Name 任务名（唯一标识，必填，用于日志和监控）
	Name string

	// Schedule 调度表达式（cron 实现支持，如 "*/5 * * * *"）
	// 内置 interval 实现忽略此字段，只用 Every
	Schedule string

	// Every 固定间隔（interval 实现支持，如 30*time.Second）
	// 内置 interval 实现必填
	Every time.Duration

	// Handler 任务执行函数（必填）
	Handler Handler

	// Timeout 单次执行超时（0 表示用 ctx 默认超时，无限制）
	// 默认 0：使用 Scheduler 传入的 ctx；非 0：包一层 WithTimeout
	Timeout time.Duration
}

// Validate 校验 Spec 字段合法性（注册前由 Scheduler 调用）
func (s Spec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("job: Spec.Name is required")
	}
	if s.Handler == nil {
		return fmt.Errorf("job: Spec.Handler is required")
	}
	// Every 和 Schedule 二者至少一个非空（具体哪个必填取决于 Scheduler 实现）
	// 这里只做软约束，由具体 Scheduler 在 Register 时强校验
	if s.Every <= 0 && s.Schedule == "" {
		return fmt.Errorf("job: Spec.Every or Spec.Schedule is required")
	}
	return nil
}

// ErrorHandler 任务执行错误处理函数。
//
// 默认实现是 log.Error，用户可注入告警/重试逻辑。
type ErrorHandler func(name string, err error)

// Scheduler 任务调度器接口。
//
// 实现者职责：
//   - Register：注册 Job Spec（在 App 启动前调用）
//   - Start：启动调度循环（阻塞或后台 goroutine，取决于实现）
//   - Stop：优雅停止（取消所有运行中的 Job 的 ctx，等待退出）
//
// 内置实现：job.NewIntervalScheduler
// 第三方实现：plugins/job/cron.NewCronScheduler
type Scheduler interface {
	// Register 注册任务规格（可批量）。
	// 同名 Job 重复注册返回 error。
	Register(specs ...Spec) error

	// Start 启动调度（非阻塞，启动后台 goroutine）。
	// 已经 Start 后再调用返回 error。
	Start(ctx context.Context) error

	// Stop 优雅停止：取消所有运行中 Job 的 ctx，等待退出。
	// 已经 Stop 后再调用是 no-op。
	Stop(ctx context.Context) error
}
