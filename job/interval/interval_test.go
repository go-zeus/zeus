package interval

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/job"
)

// TestSpec_Validate 校验逻辑
func TestSpec_Validate(t *testing.T) {
	cases := []struct {
		name    string
		spec    job.Spec
		wantErr bool
	}{
		{"valid", job.Spec{Name: "x", Every: time.Second, Handler: func(context.Context) error { return nil }}, false},
		{"missing name", job.Spec{Every: time.Second, Handler: func(context.Context) error { return nil }}, true},
		{"missing handler", job.Spec{Name: "x", Every: time.Second}, true},
		{"missing every", job.Spec{Name: "x", Handler: func(context.Context) error { return nil }}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.spec.Validate()
			if c.wantErr && err == nil {
				t.Error("expected error")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestRegister_ValidationFailed 注册时强校验
func TestRegister_ValidationFailed(t *testing.T) {
	s := New()
	// 缺 Every
	err := s.Register(job.Spec{
		Name:    "missing-every",
		Handler: func(context.Context) error { return nil },
	})
	if err == nil {
		t.Fatal("should error when Every is 0")
	}
}

// TestRegister_DuplicateName 同名重复注册报错
func TestRegister_DuplicateName(t *testing.T) {
	s := New()
	spec := job.Spec{Name: "dup", Every: time.Second, Handler: func(context.Context) error { return nil }}
	if err := s.Register(spec); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := s.Register(spec); err == nil {
		t.Fatal("second register should error")
	}
}

// TestRegister_AfterStart 启动后不能注册
func TestRegister_AfterStart(t *testing.T) {
	s := New()
	_ = s.Register(job.Spec{
		Name: "x", Every: time.Hour, Handler: func(context.Context) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err := s.Register(job.Spec{
		Name: "y", Every: time.Second, Handler: func(context.Context) error { return nil },
	})
	if err == nil {
		t.Fatal("should error when registering after Start")
	}
}

// TestStart_DoubleStartError 重复 Start 报错
func TestStart_DoubleStartError(t *testing.T) {
	s := New()
	_ = s.Register(job.Spec{
		Name: "x", Every: time.Hour, Handler: func(context.Context) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := s.Start(ctx); err == nil {
		t.Fatal("second Start should error")
	}
}

// TestScheduler_ExecuteImmediately 注册后立即执行第一次
//
// 关键契约：interval 实现首次执行不延迟一个周期
func TestScheduler_ExecuteImmediately(t *testing.T) {
	s := New()
	var count int32
	_ = s.Register(job.Spec{
		Name:  "fast",
		Every: time.Hour, // 设很长，避免 tick 期间触发
		Handler: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 等待首次执行
	deadline := time.After(time.Second)
	for {
		if atomic.LoadInt32(&count) >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("first execution did not happen within 1s, count=%d", atomic.LoadInt32(&count))
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	_ = s.Stop(context.Background())
	cancel()
}

// TestScheduler_PeriodicExecution 周期性执行（多次 tick）
func TestScheduler_PeriodicExecution(t *testing.T) {
	s := New()
	var count int32
	_ = s.Register(job.Spec{
		Name:  "tick",
		Every: 30 * time.Millisecond,
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

	// 等待至少 3 次触发（1 次立即 + 2 次 tick）
	time.Sleep(100 * time.Millisecond)

	_ = s.Stop(context.Background())

	got := atomic.LoadInt32(&count)
	if got < 3 {
		t.Errorf("expected >= 3 executions, got %d", got)
	}
}

// TestScheduler_StopGraceful Stop 后所有 goroutine 退出
func TestScheduler_StopGraceful(t *testing.T) {
	s := New()
	var count int32
	_ = s.Register(job.Spec{
		Name:  "x",
		Every: 20 * time.Millisecond,
		Handler: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	countAfterStop := atomic.LoadInt32(&count)
	// 等待 100ms 确认没有新执行
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&count) != countAfterStop {
		t.Errorf("count changed after Stop: %d → %d", countAfterStop, atomic.LoadInt32(&count))
	}
}

// TestScheduler_StopIdempotent Stop 多次调用安全
func TestScheduler_StopIdempotent(t *testing.T) {
	s := New()
	_ = s.Register(job.Spec{
		Name: "x", Every: time.Hour,
		Handler: func(context.Context) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("second Stop should be no-op, got: %v", err)
	}
}

// TestScheduler_ErrorHandler Job 返回 error 时调用 ErrorHandler
func TestScheduler_ErrorHandler(t *testing.T) {
	var handledName string
	var handledErr error
	var mu sync.Mutex

	s := New(WithErrorHandler(func(name string, err error) {
		mu.Lock()
		defer mu.Unlock()
		handledName = name
		handledErr = err
	}))

	jobErr := errors.New("simulated failure")
	_ = s.Register(job.Spec{
		Name:    "fail",
		Every:   time.Hour, // 只测试首次立即执行
		Handler: func(context.Context) error { return jobErr },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)

	// 等待 ErrorHandler 被调用
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		done := handledName == "fail"
		mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			t.Fatal("ErrorHandler not called within 1s")
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if !errors.Is(handledErr, jobErr) {
		t.Errorf("handled err = %v, want %v", handledErr, jobErr)
	}
	_ = s.Stop(context.Background())
}

// TestScheduler_Timeout Job 超时被取消
func TestScheduler_Timeout(t *testing.T) {
	s := New()
	var gotErr error
	var mu sync.Mutex

	_ = s.Register(job.Spec{
		Name:    "slow",
		Every:   time.Hour, // 防止 tick
		Timeout: 50 * time.Millisecond,
		Handler: func(ctx context.Context) error {
			<-ctx.Done() // 等 timeout 取消
			return ctx.Err()
		},
	})

	// 用自定义 ErrorHandler 捕获错误（context.DeadlineExceeded 应被吞）
	s2 := New(WithErrorHandler(func(name string, err error) {
		mu.Lock()
		defer mu.Unlock()
		gotErr = err
	}))
	_ = s2.Register(job.Spec{
		Name:    "slow",
		Every:   time.Hour,
		Timeout: 50 * time.Millisecond,
		Handler: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s2.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	_ = s2.Stop(context.Background())

	// 注：execute 会过滤 context.Canceled，但 DeadlineExceeded 不过滤
	// 这是合理的：DeadlineExceeded 表示 Job 超时，应记录；Canceled 表示正常关闭
	mu.Lock()
	defer mu.Unlock()
	if gotErr == nil {
		t.Error("expected DeadlineExceeded to be handled")
	}
	if !errors.Is(gotErr, context.DeadlineExceeded) {
		t.Errorf("got err = %v, want DeadlineExceeded", gotErr)
	}

	_ = s // 避免 unused
}

// TestScheduler_CtxCancel Stop 之外的 ctx 取消也能让 Job 退出
func TestScheduler_CtxCancel(t *testing.T) {
	s := New()
	var count int32
	_ = s.Register(job.Spec{
		Name: "x", Every: 20 * time.Millisecond,
		Handler: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel() // 直接取消 Start 的 ctx

	// 等待 goroutine 退出（ctx 取消后立即退出）
	time.Sleep(50 * time.Millisecond)

	countAfterCancel := atomic.LoadInt32(&count)
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&count) != countAfterCancel {
		t.Errorf("count changed after ctx cancel: %d → %d",
			countAfterCancel, atomic.LoadInt32(&count))
	}
}

// TestScheduler_ConcurrentJobs 多 Job 并发执行
func TestScheduler_ConcurrentJobs(t *testing.T) {
	s := New()
	var counter int32
	for i := 0; i < 3; i++ {
		name := string(rune('a' + i))
		_ = s.Register(job.Spec{
			Name:  name,
			Every: 20 * time.Millisecond,
			Handler: func(context.Context) error {
				atomic.AddInt32(&counter, 1)
				return nil
			},
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	_ = s.Stop(context.Background())

	// 3 个 Job × (1 立即 + 4 ticks) ≈ 15 次，宽松验证 >= 10
	if got := atomic.LoadInt32(&counter); got < 10 {
		t.Errorf("expected >= 10 total executions across 3 jobs, got %d", got)
	}
}
