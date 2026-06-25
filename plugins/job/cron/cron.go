// Package cron 提供基于 robfig/cron/v3 的 cron 表达式任务调度器。
//
// 设计目的：
//   - 让"工作日凌晨 2 点清理数据"、"每天 0 点跑报表"等复杂调度规则声明式接入
//   - 与内置 job/interval（固定间隔）互补，覆盖 80%/20% 调度场景
//   - 主包零依赖，本插件仅在用户需要 cron 表达式时引入
//
// 与 interval 的差异：
//   - interval 用 Spec.Every 字段（固定间隔如 30s / 5m）
//   - cron 用 Spec.Schedule 字段（cron 表达式如 "0 2 * * 1-5"）
//   - 二者读不同字段，互不冲突，可在同一 Spec 列表混用
//
// 安全默认：
//   - cron.Recover：单 Job panic 不影响其他 Job（与 interval goroutine 隔离一致）
//   - cron.SkipIfStillRunning：避免重叠执行（cron 默认会并发触发，对慢任务危险）
//   - 用户可通过 WithoutRecovery / WithoutSkipConcurrent 关闭
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/components"
//	    "github.com/go-zeus/zeus/job"
//	    cron "github.com/go-zeus/zeus/plugins/job/cron"
//	)
//
//	app := components.NewApp(
//	    components.NewJobComponent(cron.New()),
//	    components.NewJobRegistration(job.Spec{
//	        Name:     "daily-report",
//	        Schedule: "0 9 * * 1-5", // 工作日 9 点
//	        Handler:  func(ctx context.Context) error { return generateReport(ctx) },
//	        Timeout:  5 * time.Minute,
//	    }),
//	)
//	app.Run()
package cron

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/go-zeus/zeus/job"
	"github.com/go-zeus/zeus/log"
)

// cronScheduler 基于 robfig/cron/v3 的调度器实现
type cronScheduler struct {
	mu         sync.Mutex
	jobs       []job.Spec // 注册顺序保存（cron 表达式无顺序语义，保留插入顺序便于排查）
	errHandler job.ErrorHandler
	opts       []cron.Option // robfig/cron 配置选项（来自 WithLocation 等）

	started bool
	stopped bool
	cron    *cron.Cron
}

// Option 配置 Scheduler
type Option func(*cronScheduler)

// WithErrorHandler 注入自定义错误处理函数（默认 log.Error）
func WithErrorHandler(h job.ErrorHandler) Option {
	return func(s *cronScheduler) {
		if h != nil {
			s.errHandler = h
		}
	}
}

// WithLocation 设置 cron 表达式默认时区（默认 time.Local）
//
// 单个 Spec 也可通过 cron 表达式前缀 "CRON_TZ=Asia/Shanghai 0 9 * * *" 覆盖
func WithLocation(loc *time.Location) Option {
	return func(s *cronScheduler) {
		if loc != nil {
			s.opts = append(s.opts, cron.WithLocation(loc))
		}
	}
}

// WithSeconds 启用 6 字段 cron 表达式（含秒）：`"*/30 * * * * *"` 而非 5 字段
//
// 默认 5 字段："分 时 日 月 周"
func WithSeconds() Option {
	return func(s *cronScheduler) {
		s.opts = append(s.opts, cron.WithSeconds())
	}
}

// WithoutRecovery 关闭 panic 恢复（默认开启）
//
// 关闭后单 Job panic 会导致整个 cron 调度器崩溃（不推荐生产使用）
func WithoutRecovery() Option {
	return func(s *cronScheduler) {
		s.opts = append(s.opts, cron.WithChain()) // 空 chain 覆盖默认 chain
	}
}

// New 创建 cron 调度器（实现 job.Scheduler 接口）
//
// 默认安全配置（用户可通过 WithoutRecovery 关闭）：
//   - cron.Recover：单 Job panic 不影响其他 Job
//   - cron.SkipIfStillRunning：避免重叠执行（慢任务会被跳过，不堆积）
//
// 注意：robfig/cron 多次 WithChain 取最后一个生效，故用户传的 chain 会覆盖默认。
func New(opts ...Option) job.Scheduler {
	s := &cronScheduler{
		errHandler: defaultErrHandler,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}

	// 默认配置前置（用户的 opts 放后面，可覆盖默认）
	logger := cron.DiscardLogger // 变量（不是函数），实现 cron.Logger 接口的 discard 实例
	// Chain 顺序至关重要：robfig/cron 的 Chain.Then 是反向包装，输入 [A, B] 实际产生 B(A(j))。
	// 期望执行顺序：SkipIfStillRunning 最外层（拿 token） → Recover 中间（捕 panic） → 用户 Job 最内层
	// 即 wrappers = [SkipIfStillRunning, Recover]，Then 反向后执行顺序 = SkipIfStillRunning(Recover(j))
	//
	// 错误顺序 [Recover, SkipIfStillRunning] 会让 Recover 在外，panic 发生在 SkipIfStillRunning 内部，
	// 导致 token 永远不释放，后续所有调用都被 skip（详见 SkipIfStillRunning 源码）。
	defaultOpts := []cron.Option{
		cron.WithLogger(logger),
		cron.WithChain(
			cron.SkipIfStillRunning(logger),
			cron.Recover(logger),
		),
	}
	s.opts = append(defaultOpts, s.opts...)

	return s
}

// defaultErrHandler 默认错误处理：log.Error
func defaultErrHandler(name string, err error) {
	log.Error("job %s failed: %v", name, err)
}

// Register 注册一个或多个任务规格
//
// 行为：
//   - 校验 Spec（Name / Handler / Schedule 必填）
//   - 同名 Job 重复注册返回 error
//   - Start 后再 Register 返回 error
func (s *cronScheduler) Register(specs ...job.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("cron: cannot register after Start")
	}
	for _, spec := range specs {
		// cron 实现特别校验 Schedule 字段（与 interval 校验 Every 互补）
		if spec.Schedule == "" {
			return fmt.Errorf("cron: Spec %q requires Schedule (cron expression)", spec.Name)
		}
		if err := spec.Validate(); err != nil {
			return err
		}
		// 检查同名 Job（cron 库的 AddFunc 返回 ID，不便按名查重）
		for _, existing := range s.jobs {
			if existing.Name == spec.Name {
				return fmt.Errorf("cron: job %q already registered", spec.Name)
			}
		}
		// 提前验证 cron 表达式（避免 Start 时才发现错误）
		// 创建临时 parser 校验（与正式 cron 实例用相同 parser 配置）
		if err := s.validateSchedule(spec.Schedule); err != nil {
			return fmt.Errorf("cron: Spec %q has invalid schedule %q: %w", spec.Name, spec.Schedule, err)
		}
		s.jobs = append(s.jobs, spec)
	}
	return nil
}

// validateSchedule 用临时 cron 实例校验表达式语法
func (s *cronScheduler) validateSchedule(schedule string) error {
	tmp := cron.New(s.opts...)
	defer tmp.Stop()

	// 用 AddFunc 校验（成功后立即 Remove）
	id, err := tmp.AddFunc(schedule, func() {})
	if err != nil {
		return err
	}
	tmp.Remove(id)
	return nil
}

// Start 启动调度（非阻塞）
func (s *cronScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("cron: already started")
	}
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("cron: cannot restart after Stop")
	}
	s.started = true

	// 构造 cron 实例并注册所有 Job
	c := cron.New(s.opts...)
	for _, spec := range s.jobs {
		// 闭包捕获 spec（避免循环变量问题）
		specCopy := spec
		_, err := c.AddFunc(specCopy.Schedule, func() {
			s.execute(ctx, specCopy)
		})
		if err != nil {
			s.mu.Unlock()
			// 已经注册的不撤销（cron.Start 还没调用，无副作用）
			return fmt.Errorf("cron: register %q failed: %w", spec.Name, err)
		}
	}
	s.cron = c
	s.mu.Unlock()

	c.Start()
	return nil
}

// execute 执行单次 Job（含超时控制 + 错误处理）
//
// 与 interval 实现对齐：errors.Is(context.Canceled) 视为正常关闭路径
func (s *cronScheduler) execute(ctx context.Context, spec job.Spec) {
	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	if err := spec.Handler(execCtx); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		s.errHandler(spec.Name, err)
	}
}

// Stop 优雅停止调度器
//
// 行为：
//   - 调用 robfig/cron 的 c.Stop()（返回 ctx，等待运行中 Job 完成）
//   - 重复调用是 no-op
//   - 等待 ctx 超时或所有 Job 完成
func (s *cronScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started || s.stopped || s.cron == nil {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	c := s.cron
	s.mu.Unlock()

	stopCtx := c.Stop()
	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("cron: Stop timed out waiting for jobs: %w", ctx.Err())
	}
}
