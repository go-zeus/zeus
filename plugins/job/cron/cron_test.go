package cron

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/job"
)

// —— 基础功能测试 ——

// TestNew_Defaults 零参数创建的 Scheduler 默认行为正常
func TestNew_Defaults(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New returned nil")
	}
}

// TestRegister_ValidSpec 合法 Spec 注册成功
func TestRegister_ValidSpec(t *testing.T) {
	s := New()
	err := s.Register(job.Spec{
		Name:     "test",
		Schedule: "*/5 * * * *",
		Handler:  func(context.Context) error { return nil },
	})
	if err != nil {
		t.Errorf("Register err = %v", err)
	}
}

// TestRegister_MissingSchedule 缺失 Schedule 字段返回错误（与 interval 校验 Every 互补）
func TestRegister_MissingSchedule(t *testing.T) {
	s := New()
	err := s.Register(job.Spec{
		Name:    "test",
		Every:   time.Second, // 故意填 Every，cron 实现应忽略并要求 Schedule
		Handler: func(context.Context) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error when Schedule is missing")
	}
}

// TestRegister_InvalidCronExpr 非法 cron 表达式提前报错（不等 Start）
func TestRegister_InvalidCronExpr(t *testing.T) {
	s := New()
	err := s.Register(job.Spec{
		Name:     "test",
		Schedule: "not a cron expr",
		Handler:  func(context.Context) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

// TestRegister_DuplicateName 同名 Spec 二次注册报错
func TestRegister_DuplicateName(t *testing.T) {
	s := New()
	spec := job.Spec{
		Name:     "dup",
		Schedule: "*/5 * * * *",
		Handler:  func(context.Context) error { return nil },
	}
	if err := s.Register(spec); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := s.Register(spec)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

// TestRegister_AfterStart Start 后再 Register 返回错误
func TestRegister_AfterStart(t *testing.T) {
	s := New()
	_ = s.Register(job.Spec{
		Name:     "x",
		Schedule: "*/5 * * * *",
		Handler:  func(context.Context) error { return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err := s.Register(job.Spec{
		Name:     "y",
		Schedule: "*/5 * * * *",
		Handler:  func(context.Context) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error when Register after Start")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)
}

// —— 调度测试 ——

// TestSchedule_Fired 定时触发 Job 执行
func TestSchedule_Fired(t *testing.T) {
	s := New()
	var count int32
	_ = s.Register(job.Spec{
		Name:     "every-second",
		Schedule: "* * * * * *", // 启用秒字段需要 WithSeconds
		Handler: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	// 用 WithSeconds 重新创建（5 字段 cron 默认不支持秒）
	s2 := New(WithSeconds())
	var count2 int32
	_ = s2.Register(job.Spec{
		Name:     "every-second",
		Schedule: "* * * * * *", // 每秒
		Handler: func(context.Context) error {
			atomic.AddInt32(&count2, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s2.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = s // 防止 unused

	// 等 2.5 秒，至少触发 2 次
	time.Sleep(2500 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s2.Stop(stopCtx)

	if got := atomic.LoadInt32(&count2); got < 2 {
		t.Errorf("count = %d, want >= 2", got)
	}
}

// TestSchedule_AtEveryDuration robfig/cron 支持 "@every Nd" 简写
//
// 注意：robfig/cron 的 ConstantDelaySchedule 强制 >= 1s（小于会被向上取整）。
// 测试用 1s 间隔，sleep 3s 应至少触发 3 次。
func TestSchedule_AtEveryDuration(t *testing.T) {
	s := New()
	var count int32
	_ = s.Register(job.Spec{
		Name:     "every-1s",
		Schedule: "@every 1s",
		Handler: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(2500 * time.Millisecond) // 期望触发约 2-3 次

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)

	if got := atomic.LoadInt32(&count); got < 2 {
		t.Errorf("count = %d, want >= 2", got)
	}
}

// TestSchedule_TimeoutSpec Spec.Timeout 生效（Handler 看到超时）
func TestSchedule_TimeoutSpec(t *testing.T) {
	s := New()
	var sawTimeout int32

	_ = s.Register(job.Spec{
		Name:     "slow",
		Schedule: "@every 1s",
		Timeout:  100 * time.Millisecond,
		Handler: func(ctx context.Context) error {
			select {
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					atomic.StoreInt32(&sawTimeout, 1)
				}
				return ctx.Err()
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 等待 1.5s，确保至少触发一次（@every 1s 第一次也要等 1s）
	time.Sleep(1500 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)

	if atomic.LoadInt32(&sawTimeout) != 1 {
		t.Error("Spec.Timeout did not trigger context.DeadlineExceeded in Handler")
	}
}

// —— 错误处理 ——

// TestSchedule_HandlerError 返回 error 走 ErrorHandler
func TestSchedule_HandlerError(t *testing.T) {
	var mu sync.Mutex
	var gotErr error
	var gotName string

	s := New(WithErrorHandler(func(name string, err error) {
		mu.Lock()
		defer mu.Unlock()
		gotName = name
		gotErr = err
	}))

	sentinel := errors.New("boom")
	_ = s.Register(job.Spec{
		Name:     "failing",
		Schedule: "@every 1s",
		Handler:  func(context.Context) error { return sentinel },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)
	time.Sleep(1500 * time.Millisecond) // 至少 1 次触发

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)

	mu.Lock()
	defer mu.Unlock()
	if gotName != "failing" {
		t.Errorf("ErrorHandler name = %q, want failing", gotName)
	}
	if !errors.Is(gotErr, sentinel) {
		t.Errorf("ErrorHandler err = %v, want sentinel", gotErr)
	}
}

// TestSchedule_PanicRecovered 单 Job panic 不影响调度器和其他 Job
//
// 验证 chain 顺序正确：SkipIfStillRunning 在外层拿 token，Recover 在内层捕 panic，
// panic 后 token 仍能放回，后续触发不被永久 skip。
func TestSchedule_PanicRecovered(t *testing.T) {
	var panicCount int32
	var okCount int32

	s := New()
	_ = s.Register(job.Spec{
		Name:     "panic-job",
		Schedule: "@every 1s",
		Handler: func(context.Context) error {
			atomic.AddInt32(&panicCount, 1)
			panic("intentional")
		},
	})
	_ = s.Register(job.Spec{
		Name:     "ok-job",
		Schedule: "@every 1s",
		Handler: func(context.Context) error {
			atomic.AddInt32(&okCount, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(2500 * time.Millisecond) // 至少 2 次触发

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)

	// 两个 Job 都应被触发多次（panic Job 没拖死 ok Job）
	if got := atomic.LoadInt32(&panicCount); got < 2 {
		t.Errorf("panic-job count = %d, want >= 2", got)
	}
	if got := atomic.LoadInt32(&okCount); got < 2 {
		t.Errorf("ok-job count = %d, want >= 2", got)
	}
}

// —— 生命周期 ——

// TestStart_Twice 二次 Start 返回错误
func TestStart_Twice(t *testing.T) {
	s := New()
	_ = s.Register(job.Spec{
		Name:     "x",
		Schedule: "*/5 * * * *",
		Handler:  func(context.Context) error { return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	err := s.Start(ctx)
	if err == nil {
		t.Fatal("expected error on second Start")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)
}

// TestStop_Idempotent 多次 Stop 不 panic
func TestStop_Idempotent(t *testing.T) {
	s := New()
	_ = s.Register(job.Spec{
		Name:     "x",
		Schedule: "*/5 * * * *",
		Handler:  func(context.Context) error { return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)
	_ = s.Stop(stopCtx) // no-op
}

// TestStop_BeforeStart Start 之前 Stop 是 no-op（返回 nil）
func TestStop_BeforeStart(t *testing.T) {
	s := New()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer stopCancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Errorf("Stop before Start err = %v", err)
	}
}

// TestStop_GracefulShutdown Stop 等待运行中的 Job 完成
//
// 验证 robfig/cron.Stop() 的 graceful 语义：不取消正在执行的 Job，
// 等其自然完成后才返回。
func TestStop_GracefulShutdown(t *testing.T) {
	s := New()
	var started int32
	var completed int32

	_ = s.Register(job.Spec{
		Name:     "long-running",
		Schedule: "@every 1s",
		Handler: func(ctx context.Context) error {
			atomic.StoreInt32(&started, 1)
			// 模拟长任务：等 800ms（不监听 ctx.Done 以验证 Stop 真的等）
			time.Sleep(800 * time.Millisecond)
			atomic.StoreInt32(&completed, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)

	// 等 1.2s 确保 @every 1s 已触发
	time.Sleep(1200 * time.Millisecond)
	if atomic.LoadInt32(&started) != 1 {
		t.Fatal("job did not start within 1.2s")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	err := s.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop err = %v", err)
	}

	if atomic.LoadInt32(&completed) != 1 {
		t.Error("Stop did not wait for in-flight job to complete")
	}
}

// —— 配置选项 ——

// TestWithLocation 自定义时区生效
func TestWithLocation(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("timezone data not available")
	}
	s := New(WithLocation(loc))
	if s == nil {
		t.Fatal("New returned nil")
	}
	// 仅验证构造无 panic（具体行为验证需要真实时钟，此处略）
}

// TestWithSeconds 6 字段表达式生效
func TestWithSeconds(t *testing.T) {
	s := New(WithSeconds())
	var count int32
	_ = s.Register(job.Spec{
		Name:     "fast",
		Schedule: "* * * * * *", // 6 字段（含秒）
		Handler: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(2500 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = s.Stop(stopCtx)

	if got := atomic.LoadInt32(&count); got < 2 {
		t.Errorf("count = %d, want >= 2", got)
	}
}
