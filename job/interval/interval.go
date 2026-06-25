// Package interval 提供基于 time.Ticker 的固定间隔任务调度器。
//
// 设计选择：
//   - 零依赖（仅用标准库 time.Ticker）
//   - 不支持 cron 表达式，只支持 Every 字段
//   - 每个 Job 独立 goroutine + Ticker（隔离故障）
//   - 单次执行默认无超时（用 Scheduler.Stop 的 ctx 控制）
//
// 局限：
//   - 不支持 cron 表达式（如 "0 0 * * 1-5" 工作日 0 点）
//   - 不支持 jitter（避免 thundering herd），用户应在 Handler 内自己 jitter
//   - 不持久化（重启后从头开始，不补跑漏掉的任务）
//
// 高级需求请用 plugins/job/cron。
package interval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-zeus/zeus/job"
	"github.com/go-zeus/zeus/log"
)

// intervalScheduler 基于 time.Ticker 的固定间隔调度器
type intervalScheduler struct {
	mu         sync.Mutex
	jobs       map[string]job.Spec // 已注册任务（key = Spec.Name）
	errHandler job.ErrorHandler
	started    bool               // 是否已 Start
	stopped    bool               // 是否已 Stop
	cancel     context.CancelFunc // Start 时创建，Stop 时调用
	wg         sync.WaitGroup     // 等待所有 Job goroutine 退出
}

// Option 配置 Scheduler
type Option func(*intervalScheduler)

// WithErrorHandler 注入自定义错误处理函数（默认 log.Error）
func WithErrorHandler(h job.ErrorHandler) Option {
	return func(s *intervalScheduler) {
		if h != nil {
			s.errHandler = h
		}
	}
}

// New 创建基于 interval 的调度器
func New(opts ...Option) job.Scheduler {
	s := &intervalScheduler{
		jobs:       make(map[string]job.Spec),
		errHandler: defaultErrHandler,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// defaultErrHandler 默认错误处理：log.Error
func defaultErrHandler(name string, err error) {
	log.Error("job %s failed: %v", name, err)
}

// Register 注册一个或多个任务规格
//
// 行为：
//   - 校验 Spec（Name / Handler / Every 必填）
//   - 同名 Job 重复注册返回 error
//   - Start 后再 Register 返回 error（避免运行时修改调度）
func (s *intervalScheduler) Register(specs ...job.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("interval: cannot register after Start")
	}
	for _, spec := range specs {
		// interval 实现特别校验 Every 字段
		if spec.Every <= 0 {
			return fmt.Errorf("interval: Spec %q requires Every > 0", spec.Name)
		}
		if err := spec.Validate(); err != nil {
			return err
		}
		if _, exists := s.jobs[spec.Name]; exists {
			return fmt.Errorf("interval: job %q already registered", spec.Name)
		}
		s.jobs[spec.Name] = spec
	}
	return nil
}

// Start 启动所有已注册任务的调度循环
//
// 行为：
//   - 非阻塞：为每个 Job 启动独立 goroutine
//   - 已经 Start 后再调用返回 error
//   - Stop 后再调用返回 error（不可重启）
//   - 每次触发：构造 ctx（含可选 Timeout）→ 调 Handler → 错误走 ErrorHandler
func (s *intervalScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("interval: already started")
	}
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("interval: cannot restart after Stop")
	}
	s.started = true
	// 派生可取消 ctx，所有 Job goroutine 共享
	startCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	jobsToStart := make([]job.Spec, 0, len(s.jobs))
	for _, spec := range s.jobs {
		jobsToStart = append(jobsToStart, spec)
	}
	s.mu.Unlock()

	for _, spec := range jobsToStart {
		s.wg.Add(1)
		go s.runJob(startCtx, spec)
	}
	return nil
}

// runJob 单个 Job 的调度循环（独立 goroutine）
func (s *intervalScheduler) runJob(ctx context.Context, spec job.Spec) {
	defer s.wg.Done()

	ticker := time.NewTicker(spec.Every)
	defer ticker.Stop()

	// 首次立即执行一次（典型场景：注册后马上跑一次，后续按间隔重复）
	// 不立即执行会延迟一个 Every 周期，对心跳类任务不友好
	s.execute(ctx, spec)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.execute(ctx, spec)
		}
	}
}

// execute 执行单次 Job（含超时控制 + 错误处理）
func (s *intervalScheduler) execute(ctx context.Context, spec job.Spec) {
	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	if err := spec.Handler(execCtx); err != nil {
		// ctx 被取消属于正常关闭路径，不算错误
		if errors.Is(err, context.Canceled) {
			return
		}
		s.errHandler(spec.Name, err)
	}
}

// Stop 优雅停止调度器
//
// 行为：
//   - 取消所有 Job goroutine 的 ctx
//   - 等待所有 Job 退出（最多等到 ctx.Done）
//   - 重复调用是 no-op
func (s *intervalScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started || s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	cancel := s.cancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	// 等待所有 Job goroutine 退出，或 ctx 超时
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("interval: Stop timed out waiting for jobs: %w", ctx.Err())
	}
}
