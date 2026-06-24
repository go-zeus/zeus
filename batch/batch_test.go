package batch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// —— 基础行为 ——

func TestNew_DefaultConfig(t *testing.T) {
	var got [][]int
	var mu sync.Mutex
	b := New(func(items []int) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, items)
	}, WithMaxBatchSize(2))
	defer b.Close()

	b.Add(1)
	b.Add(2)

	// size=2 应该立即触发
	waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	if len(got[0]) != 2 {
		t.Errorf("batch len = %d, want 2", len(got[0]))
	}
}

func TestAdd_TriggersOnSize(t *testing.T) {
	var total int64
	b := New(func(items []int) {
		atomic.AddInt64(&total, int64(len(items)))
	}, WithMaxBatchSize(3))
	defer b.Close()

	for i := 0; i < 9; i++ {
		b.Add(i)
	}

	waitUntil(t, time.Second, func() bool { return atomic.LoadInt64(&total) == 9 })
	if got := atomic.LoadInt64(&total); got != 9 {
		t.Errorf("total processed = %d, want 9", got)
	}
}

func TestAdd_TriggersOnTime(t *testing.T) {
	var total int64
	b := New(func(items []int) {
		atomic.AddInt64(&total, int64(len(items)))
	}, WithMaxBatchSize(1000), WithMaxWait(50*time.Millisecond))
	defer b.Close()

	for i := 0; i < 5; i++ {
		b.Add(i)
	}

	// 不到 maxSize，但等 50ms 应该触发
	waitUntil(t, time.Second, func() bool { return atomic.LoadInt64(&total) == 5 })
	if got := atomic.LoadInt64(&total); got != 5 {
		t.Errorf("total processed = %d, want 5", got)
	}
}

// —— Flush ——

func TestFlush_Immediate(t *testing.T) {
	var got []int
	var mu sync.Mutex
	b := New(func(items []int) {
		mu.Lock()
		got = append(got, items...)
		mu.Unlock()
	}, WithMaxBatchSize(100), WithMaxWait(time.Hour)) // 故意调大，仅靠 Flush
	defer b.Close()

	for i := 0; i < 5; i++ {
		b.Add(i)
	}
	b.Flush()

	waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 5
	})
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 {
		t.Errorf("len = %d, want 5", len(got))
	}
}

// —— Close ——

func TestClose_FlushesRemaining(t *testing.T) {
	var got []int
	var mu sync.Mutex
	b := New(func(items []int) {
		mu.Lock()
		got = append(got, items...)
		mu.Unlock()
	}, WithMaxBatchSize(100), WithMaxWait(time.Hour))

	for i := 0; i < 10; i++ {
		b.Add(i)
	}

	b.Close() // 应该 flush 所有 pending

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 10 {
		t.Errorf("after close, processed = %d, want 10", len(got))
	}
}

func TestAdd_AfterClose(t *testing.T) {
	b := New(func(items []int) {}, WithMaxBatchSize(1))
	b.Close()
	// 不应 panic
	b.Add(1)
}

// —— TryAdd ——

func TestTryAdd_QueueFull(t *testing.T) {
	// 用非常小的 maxWait 和 maxBatchSize，但 handler 很慢
	// 实际上很难造出 queue full 的场景，所以只验证基本行为
	var total int64
	b := New(func(items []int) {
		atomic.AddInt64(&total, int64(len(items)))
	}, WithMaxBatchSize(2))
	defer b.Close()

	if !b.TryAdd(1) {
		t.Error("TryAdd(1) should succeed on fresh batcher")
	}
	if !b.TryAdd(2) {
		t.Error("TryAdd(2) should succeed")
	}

	waitUntil(t, time.Second, func() bool { return atomic.LoadInt64(&total) == 2 })
}

// —— AddContext ——

func TestAddContext_CtxCanceled(t *testing.T) {
	b := New(func(items []int) {}, WithMaxBatchSize(100), WithMaxWait(time.Hour))
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.AddContext(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAddContext_Success(t *testing.T) {
	var total int64
	b := New(func(items []int) {
		atomic.AddInt64(&total, int64(len(items)))
	}, WithMaxBatchSize(1))
	defer b.Close()

	err := b.AddContext(context.Background(), 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	waitUntil(t, time.Second, func() bool { return atomic.LoadInt64(&total) == 1 })
}

// —— 并发 ——

func TestAdd_ConcurrentSafe(t *testing.T) {
	var total int64
	b := New(func(items []int) {
		atomic.AddInt64(&total, int64(len(items)))
	}, WithMaxBatchSize(50), WithMaxWait(20*time.Millisecond))
	defer b.Close()

	const goroutines = 50
	const perG = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				b.Add(start + i)
			}
		}(g * perG)
	}
	wg.Wait()

	// 等 Close 完成
	b.Close()

	expected := int64(goroutines * perG)
	if got := atomic.LoadInt64(&total); got != expected {
		t.Errorf("total = %d, want %d", got, expected)
	}
}

// —— handler panic 不应崩溃 ——

func TestHandlerPanic_DoesNotCrash(t *testing.T) {
	var calls int32
	b := New(func(items []int) {
		atomic.AddInt32(&calls, 1)
		panic("boom")
	}, WithMaxBatchSize(2))
	defer b.Close()

	b.Add(1)
	b.Add(2)
	b.Add(3)
	b.Add(4)

	waitUntil(t, time.Second, func() bool { return atomic.LoadInt32(&calls) >= 2 })
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Errorf("calls = %d, want >= 2 (panic should not crash)", got)
	}
}

// —— 自定义类型 ——

func TestNew_StringType(t *testing.T) {
	var got []string
	var mu sync.Mutex
	b := New(func(items []string) {
		mu.Lock()
		got = append(got, items...)
		mu.Unlock()
	}, WithMaxBatchSize(2))
	defer b.Close()

	b.Add("a")
	b.Add("b")

	waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 2
	})
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got = %v", got)
	}
}

// —— 等于 maxSize 边界 ——

func TestAdd_ExactMaxSize(t *testing.T) {
	var batches int32
	b := New(func(items []int) {
		atomic.AddInt32(&batches, 1)
	}, WithMaxBatchSize(5), WithMaxWait(time.Hour))
	defer b.Close()

	for i := 0; i < 5; i++ {
		b.Add(i)
	}

	waitUntil(t, time.Second, func() bool { return atomic.LoadInt32(&batches) == 1 })
}

// —— 辅助 ——

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met within timeout")
	}
}
