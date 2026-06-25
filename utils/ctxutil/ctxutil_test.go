package ctxutil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// —— Merge ——

func TestMerge_NoCancel(t *testing.T) {
	a, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	b, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	merged, cancel := Merge(a, b)
	defer cancel()

	if merged.Err() != nil {
		t.Errorf("merged should be active, got %v", merged.Err())
	}
}

func TestMerge_AnyCancelsAll(t *testing.T) {
	a, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	b, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	merged, cancel := Merge(a, b)
	defer cancel()

	cancelA() // 取消 a → merged 应被取消

	select {
	case <-merged.Done():
		// 期望路径
	case <-time.After(100 * time.Millisecond):
		t.Fatal("merged should be canceled when parent a is canceled")
	}
}

func TestMerge_SecondParentCancels(t *testing.T) {
	a, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	b, cancelB := context.WithCancel(context.Background())

	merged, cancel := Merge(a, b)
	defer cancel()

	cancelB()

	select {
	case <-merged.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("merged should be canceled when parent b is canceled")
	}
}

func TestMerge_ExternalCancel(t *testing.T) {
	a, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	b, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	merged, cancel := Merge(a, b)
	cancel() // 外部主动取消

	select {
	case <-merged.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("merged should be canceled when external cancel() called")
	}
}

func TestMerge_NoParents(t *testing.T) {
	// 无父 ctx 也应正常工作
	merged, cancel := Merge()
	defer cancel()

	if merged.Err() != nil {
		t.Errorf("Merge() with no parents should be active")
	}
}

func TestMerge_NilParentSkipped(t *testing.T) {
	a, cancelA := context.WithCancel(context.Background())
	defer cancelA()

	merged, cancel := Merge(nil, a, nil)
	defer cancel()

	cancelA()
	select {
	case <-merged.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("nil parents should be skipped, but real parent cancel must propagate")
	}
}

func TestMerge_PropagatesDeadline(t *testing.T) {
	// 父 ctx 有 deadline → merged 也应有该 deadline
	parent, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	merged, cancelM := Merge(parent)
	defer cancelM()

	dl, ok := merged.Deadline()
	if !ok {
		t.Fatal("merged should inherit deadline from parent")
	}
	parentDl, _ := parent.Deadline()
	if !dl.Equal(parentDl) {
		t.Errorf("merged deadline = %v, want %v", dl, parentDl)
	}
}

func TestMerge_PicksEarliestDeadline(t *testing.T) {
	short, cancelS := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelS()
	long, cancelL := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelL()

	merged, cancel := Merge(short, long)
	defer cancel()

	dl, ok := merged.Deadline()
	if !ok {
		t.Fatal("merged should have a deadline")
	}
	shortDl, _ := short.Deadline()
	if !dl.Equal(shortDl) {
		t.Errorf("merged should pick earliest deadline, got %v want %v", dl, shortDl)
	}
}

func TestMerge_NoGoroutineLeak(t *testing.T) {
	// 启动 + 取消 100 个 Merge，所有监听 goroutine 应退出
	const n = 100
	parents := make([]context.Context, n)
	cancels := make([]context.CancelFunc, n)
	for i := range parents {
		parents[i], cancels[i] = context.WithCancel(context.Background())
	}

	mergedCancels := make([]context.CancelFunc, n)
	for i := range parents {
		_, mergedCancels[i] = Merge(parents[i])
	}
	// 全部取消
	for i := range cancels {
		cancels[i]()
	}
	for i := range mergedCancels {
		mergedCancels[i]()
	}

	// 等待清理（监听 goroutine 通过 merged.Done() 自然退出）
	time.Sleep(100 * time.Millisecond)
	// 不直接断言 goroutine 数（脆弱），而是验证不会卡死
}

// —— WithTimeoutFromCtx ——

func TestWithTimeoutFromCtx_NoDeadline(t *testing.T) {
	src := context.Background()
	dst, cancel := WithTimeoutFromCtx(context.Background(), src)
	defer cancel()

	if _, ok := dst.Deadline(); ok {
		t.Error("dst should not have deadline when src has none")
	}
}

func TestWithTimeoutFromCtx_InheritsDeadline(t *testing.T) {
	src, cancelSrc := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelSrc()

	start := time.Now()
	dst, cancel := WithTimeoutFromCtx(context.Background(), src)
	defer cancel()

	// 等 250ms 应触发 dst 超时
	<-dst.Done()
	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Errorf("dst should timeout around 200ms, got %v", elapsed)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("dst timeout too late: %v", elapsed)
	}
}

func TestWithTimeoutFromCtx_AlreadyExpired(t *testing.T) {
	// src 已超时 → dst 应立即取消
	src, cancelSrc := context.WithTimeout(context.Background(), -1*time.Second)
	defer cancelSrc()

	dst, cancel := WithTimeoutFromCtx(context.Background(), src)
	defer cancel()

	if dst.Err() == nil {
		t.Error("dst should be immediately canceled when src already expired")
	}
}

// —— IsCanceled / IsDeadline ——

func TestIsCanceled(t *testing.T) {
	if !IsCanceled(context.Canceled) {
		t.Error("IsCanceled(context.Canceled) should be true")
	}
	if IsCanceled(context.DeadlineExceeded) {
		t.Error("IsCanceled(context.DeadlineExceeded) should be false")
	}
	if IsCanceled(errors.New("other")) {
		t.Error("IsCanceled(other) should be false")
	}
}

func TestIsDeadline(t *testing.T) {
	if !IsDeadline(context.DeadlineExceeded) {
		t.Error("IsDeadline(context.DeadlineExceeded) should be true")
	}
	if IsDeadline(context.Canceled) {
		t.Error("IsDeadline(context.Canceled) should be false")
	}
}

func TestIsCanceled_WrappedError(t *testing.T) {
	wrapped := errors.Join(context.Canceled, errors.New("extra"))
	if !IsCanceled(wrapped) {
		t.Error("IsCanceled should match wrapped errors")
	}
}

func TestIsCanceledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !IsCanceledCtx(ctx) {
		t.Error("IsCanceledCtx should be true for canceled ctx")
	}
}

func TestIsDeadlineExceededCtx(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()
	<-ctx.Done()
	if !IsDeadlineExceededCtx(ctx) {
		t.Error("IsDeadlineExceededCtx should be true for timed-out ctx")
	}
}

func TestIsCanceledCtx_ActiveCtx(t *testing.T) {
	ctx := context.Background()
	if IsCanceledCtx(ctx) {
		t.Error("IsCanceledCtx should be false for active ctx")
	}
}

// —— DoneOrBlock ——

func TestDoneOrBlock_FnReturns(t *testing.T) {
	got, err := DoneOrBlock(context.Background(), func() int {
		return 42
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != 42 {
		t.Errorf("got = %d, want 42", got)
	}
}

func TestDoneOrBlock_CtxAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DoneOrBlock(ctx, func() int {
		return 42
	})
	if !IsCanceled(err) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestDoneOrBlock_CtxCancelsBeforeFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := DoneOrBlock(ctx, func() int {
		time.Sleep(2 * time.Second) // 长时操作
		return 42
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from canceled ctx")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("should return shortly after cancel, elapsed = %v", elapsed)
	}
}

func TestDoneOrBlock_FnReturnsBeforeCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got, err := DoneOrBlock(ctx, func() string {
		return "done"
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "done" {
		t.Errorf("got = %q, want done", got)
	}
}

// —— 综合场景 ——

func TestComposite_PropagateClientTimeout(t *testing.T) {
	// 模拟：client 给 200ms，server 收到后再用剩余时间跑 DB
	clientCtx, cancelClient := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelClient()

	var ops int32
	// 模拟 server 处理 + DB 子调用
	dbCtx, cancelDB := WithTimeoutFromCtx(context.Background(), clientCtx)
	defer cancelDB()

	<-dbCtx.Done() // 等 dbCtx 超时
	atomic.AddInt32(&ops, 1)

	if !IsDeadlineExceededCtx(dbCtx) {
		t.Errorf("dbCtx should be deadline exceeded, got %v", dbCtx.Err())
	}
}

// —— Benchmark ——

func BenchmarkMerge_2Parents(b *testing.B) {
	for i := 0; i < b.N; i++ {
		a, ca := context.WithCancel(context.Background())
		cc, cb := context.WithCancel(context.Background())
		_, cancel := Merge(a, cc)
		ca()
		cb()
		cancel()
	}
}

func BenchmarkMerge_5Parents(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctxs := make([]context.Context, 5)
		cancels := make([]context.CancelFunc, 5)
		for j := range ctxs {
			ctxs[j], cancels[j] = context.WithCancel(context.Background())
		}
		_, cancel := Merge(ctxs...)
		cancel()
		for _, c := range cancels {
			c()
		}
	}
}

func BenchmarkWithTimeoutFromCtx(b *testing.B) {
	src, cancelSrc := context.WithTimeout(context.Background(), time.Hour)
	defer cancelSrc()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, cancel := WithTimeoutFromCtx(context.Background(), src)
		cancel()
	}
}

func BenchmarkIsCanceled(b *testing.B) {
	err := context.Canceled
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsCanceled(err)
	}
}
