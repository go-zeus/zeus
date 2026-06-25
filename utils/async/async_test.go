package async_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/utils/async"
)

type Te string

func (t Te) Echo() string {
	return string(t)
}

func testAsync() Te {
	return "hello async"
}

func testAsync2() Te {
	time.Sleep(300 * time.Millisecond)
	return "hello async2"
}

func TestAwait(t *testing.T) {
	val := async.Exec(testAsync)
	val2 := async.Exec(testAsync2)

	a, _ := val.Await()
	if a != "hello async" {
		t.Errorf("got %q, want %q", a, "hello async")
	}

	b, _ := val2.Await()
	if b != "hello async2" {
		t.Errorf("got %q, want %q", b, "hello async2")
	}
}

// TestAwaitCtx_TimeoutAwait AwaitCtx 在 ctx 超时后立即返回
func TestAwaitCtx_TimeoutAwait(t *testing.T) {
	fut := async.Exec(func() string {
		time.Sleep(500 * time.Millisecond)
		return "slow"
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := fut.AwaitCtx(ctx)
	if err == nil {
		t.Fatal("expected ctx.Err() on AwaitCtx timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

// TestExec_PanicRecovered panic 转为 error
func TestExec_PanicRecovered(t *testing.T) {
	fut := async.Exec(func() string {
		panic("intentional panic")
	})
	_, err := fut.Await()
	if err == nil {
		t.Fatal("expected error from recovered panic")
	}
}

// TestExecCtx_Basic 正常路径
func TestExecCtx_Basic(t *testing.T) {
	fut := async.ExecCtx(context.Background(), func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	v, err := fut.Await()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v != "ok" {
		t.Errorf("v = %q, want ok", v)
	}
}

// TestExecCtx_CancelAwait AwaitCtx 取消时立即返回 + 通知 goroutine 退出
//
// 这是 P0-5 修复的核心：ExecCtx 路径下取消 AwaitCtx 应同时通知后台 goroutine
func TestExecCtx_CancelAwait(t *testing.T) {
	var goroutineStarted int32
	var goroutineExited int32

	fut := async.ExecCtx(context.Background(), func(ctx context.Context) (string, error) {
		atomic.StoreInt32(&goroutineStarted, 1)
		<-ctx.Done() // 模拟长时操作 + 监听 ctx
		atomic.StoreInt32(&goroutineExited, 1)
		return "", ctx.Err()
	})

	// 等待 goroutine 起来
	if !waitForCondition(time.Second, func() bool {
		return atomic.LoadInt32(&goroutineStarted) == 1
	}) {
		t.Fatal("goroutine did not start")
	}

	// 超时取消
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _ = fut.AwaitCtx(ctx)

	// goroutine 应通过 ctx.Done() 感知取消并退出
	if !waitForCondition(time.Second, func() bool {
		return atomic.LoadInt32(&goroutineExited) == 1
	}) {
		t.Fatal("goroutine did not exit after ctx cancel (potential leak)")
	}
}

// TestExecCtx_ExplicitCancel Cancel() 显式取消
func TestExecCtx_ExplicitCancel(t *testing.T) {
	var exited int32

	fut := async.ExecCtx(context.Background(), func(ctx context.Context) (string, error) {
		<-ctx.Done()
		atomic.StoreInt32(&exited, 1)
		return "", ctx.Err()
	})

	fut.Cancel()

	if !waitForCondition(time.Second, func() bool {
		return atomic.LoadInt32(&exited) == 1
	}) {
		t.Fatal("Cancel() did not unblock the goroutine")
	}
}

// TestExec_Cancel_NoOp Exec 路径的 Cancel() 应是 no-op（不 panic）
func TestExec_Cancel_NoOp(t *testing.T) {
	fut := async.Exec(func() string { return "ok" })
	// 不应 panic
	fut.Cancel()
	v, err := fut.Await()
	if err != nil || v != "ok" {
		t.Errorf("Exec future after Cancel: v=%q err=%v", v, err)
	}
}

// TestExecCtx_PanicRecovered ExecCtx 也能 recover panic
func TestExecCtx_PanicRecovered(t *testing.T) {
	fut := async.ExecCtx(context.Background(), func(ctx context.Context) (string, error) {
		panic("intentional")
	})
	_, err := fut.Await()
	if err == nil {
		t.Fatal("expected error from recovered panic")
	}
}

// TestExecCtx_NoLeakUnderHighConcurrency 高并发场景无 goroutine 泄漏
//
// 启动 100 个 ExecCtx，全部立即 Cancel，确认所有 goroutine 都退出
func TestExecCtx_NoLeakUnderHighConcurrency(t *testing.T) {
	const n = 100
	var activeCount int32

	futs := make([]async.Future[string], n)
	for i := 0; i < n; i++ {
		futs[i] = async.ExecCtx(context.Background(), func(ctx context.Context) (string, error) {
			atomic.AddInt32(&activeCount, 1)
			defer atomic.AddInt32(&activeCount, -1)
			<-ctx.Done()
			return "", ctx.Err()
		})
	}

	// 等所有 goroutine 起来
	if !waitForCondition(2*time.Second, func() bool {
		return atomic.LoadInt32(&activeCount) == int32(n)
	}) {
		t.Fatalf("not all goroutines started: active = %d", atomic.LoadInt32(&activeCount))
	}

	// 全部取消
	for _, f := range futs {
		f.Cancel()
	}

	// 等所有 goroutine 退出
	if !waitForCondition(2*time.Second, func() bool {
		return atomic.LoadInt32(&activeCount) == 0
	}) {
		t.Fatalf("goroutines leaked: active = %d", atomic.LoadInt32(&activeCount))
	}
}

// TestGoroutineCountStable 跑前后 goroutine 数应稳定（无泄漏）
//
// 这是更严格的泄漏测试：通过 runtime.NumGoroutine 检查
func TestGoroutineCountStable(t *testing.T) {
	// 先 warmup（async.Exec 内部 channel 等可能短暂持有 goroutine）
	for i := 0; i < 10; i++ {
		f := async.Exec(func() int { return i })
		_, _ = f.Await()
	}
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)

	before := runtime.NumGoroutine()

	// 启动 + 取消 200 个 ExecCtx
	for i := 0; i < 200; i++ {
		f := async.ExecCtx(context.Background(), func(ctx context.Context) (int, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		})
		f.Cancel()
	}

	// 等待清理
	if !waitForCondition(3*time.Second, func() bool {
		return runtime.NumGoroutine() <= before+5 // 容忍少量误差
	}) {
		t.Errorf("goroutine leaked: before=%d, after=%d", before, runtime.NumGoroutine())
	}
}

// waitForCondition 轮询等待条件满足（避免 sleep 硬编码）
func waitForCondition(max time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}
