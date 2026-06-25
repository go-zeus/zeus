package components

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/job"
	"github.com/go-zeus/zeus/job/interval"
)

// mockScheduler 用于测试 JobComponent 是否正确编排
type mockScheduler struct {
	mu          sync.Mutex
	registered  []job.Spec
	started     bool
	stopped     bool
	startErr    error
	stopErr     error
	registerErr error
}

func (m *mockScheduler) Register(specs ...job.Spec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registerErr != nil {
		return m.registerErr
	}
	m.registered = append(m.registered, specs...)
	return nil
}

func (m *mockScheduler) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockScheduler) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	return nil
}

// TestJobComponent_Lifecycle JobComponent.OnStart 收集 + 注册 + 启动（用 Container 驱动）
func TestJobComponent_Lifecycle(t *testing.T) {
	mock := &mockScheduler{}
	jc := NewJobComponent(mock)

	spec1 := job.Spec{Name: "a", Every: time.Second, Handler: func(context.Context) error { return nil }}
	spec2 := job.Spec{Name: "b", Every: time.Second, Handler: func(context.Context) error { return nil }}

	c := NewContainer()
	_ = c.Register(jc)
	_ = c.Register(NewJobRegistration(spec1))
	_ = c.Register(NewJobRegistration(spec2))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Container.Start: %v", err)
	}

	if !mock.started {
		t.Error("Scheduler.Start should be called")
	}
	if len(mock.registered) != 2 {
		t.Errorf("registered %d specs, want 2", len(mock.registered))
	}

	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Container.Stop: %v", err)
	}
	if !mock.stopped {
		t.Error("Scheduler.Stop should be called")
	}
}

// TestJobComponent_Integration 完整端到端：Container 启动后 Job 自动执行
//
// 用真实 interval.Scheduler + Container.Start 验证声明式注册 + 自动调度的完整链路
func TestJobComponent_Integration(t *testing.T) {
	var count1, count2 int32

	jc := NewJobComponent(interval.New())
	reg1 := NewJobRegistration(job.Spec{
		Name:    "job1",
		Every:   30 * time.Millisecond,
		Handler: func(context.Context) error { atomic.AddInt32(&count1, 1); return nil },
	})
	reg2 := NewJobRegistration(job.Spec{
		Name:    "job2",
		Every:   50 * time.Millisecond,
		Handler: func(context.Context) error { atomic.AddInt32(&count2, 1); return nil },
	})

	c := NewContainer()
	_ = c.Register(jc)
	_ = c.Register(reg1)
	_ = c.Register(reg2)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Container.Start: %v", err)
	}

	// 等待 Job 触发
	time.Sleep(100 * time.Millisecond)

	// OnStop 优雅关闭
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Container.Stop: %v", err)
	}

	if atomic.LoadInt32(&count1) == 0 {
		t.Error("job1 never executed")
	}
	if atomic.LoadInt32(&count2) == 0 {
		t.Error("job2 never executed")
	}
}

// TestJobComponent_NoScheduler Scheduler 为 nil 时 no-op
func TestJobComponent_NoScheduler(t *testing.T) {
	jc := NewJobComponent(nil)
	c := NewContainer()
	_ = c.Register(jc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start with nil scheduler should be no-op, got: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop with nil scheduler should be no-op, got: %v", err)
	}
}

// TestJobComponent_StartError Scheduler.Start 失败时 Container.Start 返回 error
func TestJobComponent_StartError(t *testing.T) {
	startErr := errors.New("start failed")
	mock := &mockScheduler{startErr: startErr}
	jc := NewJobComponent(mock)

	c := NewContainer()
	_ = c.Register(jc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Scheduler.Start")
	}
	if !errors.Is(err, startErr) {
		t.Errorf("err = %v, want %v", err, startErr)
	}
}

// TestJobRegistration_Name 包含 Spec.Name 用于诊断
func TestJobRegistration_Name(t *testing.T) {
	reg := NewJobRegistration(job.Spec{Name: "my-task"})
	if reg.Name() != "job_registration:my-task" {
		t.Errorf("Name = %q, want job_registration:my-task", reg.Name())
	}
}

// TestJobRegistration_DependsOnJob 依赖 job 组件（保证 Scheduler 先就绪）
func TestJobRegistration_DependsOnJob(t *testing.T) {
	reg := NewJobRegistration(job.Spec{Name: "x"})
	deps := reg.Depends()
	if len(deps) != 1 || deps[0] != "job" {
		t.Errorf("Depends = %v, want [job]", deps)
	}
}
