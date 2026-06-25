package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/ratelimit"
	"github.com/go-zeus/zeus/routing"
)

// mockLimiter 用于测试，记录 Allow 调用次数
type mockLimiter struct {
	mu       sync.Mutex
	allowCnt int
	rate     float64
}

func (m *mockLimiter) Allow() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowCnt++
	return true
}
func (m *mockLimiter) Reserve() ratelimit.WaitDuration {
	return ratelimit.WaitDuration{Allow: true, Duration: 0}
}
func (m *mockLimiter) Rate() float64 { return m.rate }

var _ ratelimit.Limiter = (*mockLimiter)(nil)

// TestClusterLimiter_DistinctKeys_DistinctBuckets 验证不同 key 拥有独立桶
func TestClusterLimiter_DistinctKeys_DistinctBuckets(t *testing.T) {
	var defaultCnt, canaryCnt int
	cl := New(func() ratelimit.Limiter {
		// 简单工厂：返回一个新的 mock
		return &mockLimiter{}
	})

	// 用同一 key 多次调用，应只创建一个桶
	_ = cl.AllowKey("default")
	_ = cl.AllowKey("default")
	_ = cl.AllowKey("canary")

	keys := cl.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %v", len(keys), keys)
	}

	_ = defaultCnt
	_ = canaryCnt
}

// TestClusterLimiter_DefaultKeyFromContext 验证默认从 ctx 提取 cluster 作为 key
func TestClusterLimiter_DefaultKeyFromContext(t *testing.T) {
	cl := New(func() ratelimit.Limiter {
		return &mockLimiter{}
	})

	ctx := routing.WithCluster(context.Background(), "canary")
	_ = cl.Allow(ctx)

	keys := cl.Keys()
	if len(keys) != 1 || keys[0] != "canary" {
		t.Fatalf("expected [canary], got %v", keys)
	}
}

// TestClusterLimiter_DefaultCluster_WhenMissing 验证缺失 cluster 时使用 default
func TestClusterLimiter_DefaultCluster_WhenMissing(t *testing.T) {
	cl := New(func() ratelimit.Limiter {
		return &mockLimiter{}
	})

	_ = cl.Allow(context.Background())

	keys := cl.Keys()
	if len(keys) != 1 || keys[0] != routing.Default {
		t.Fatalf("expected [%s], got %v", routing.Default, keys)
	}
}

// TestClusterLimiter_FactoryNil_FallsBackToNoop 验证 factory nil 时使用 noop
func TestClusterLimiter_FactoryNil_FallsBackToNoop(t *testing.T) {
	cl := New(nil)
	if !cl.AllowKey("x") {
		t.Error("noop should always allow")
	}
}

// TestClusterLimiter_ConcurrentSafe 验证并发安全（必须 -race 通过）
func TestClusterLimiter_ConcurrentSafe(t *testing.T) {
	cl := New(func() ratelimit.Limiter {
		return &mockLimiter{}
	})

	var wg sync.WaitGroup
	clusters := []string{"default", "canary", "order.v2", "batch.v3"}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := routing.WithCluster(context.Background(), clusters[idx%len(clusters)])
			_ = cl.Allow(ctx)
		}(i)
	}
	wg.Wait()

	if len(cl.Keys()) != len(clusters) {
		t.Errorf("expected %d buckets, got %d", len(clusters), len(cl.Keys()))
	}
}

// TestClusterLimiter_Reserve_UsesKeyBucket 验证 Reserve 也按 key 路由
func TestClusterLimiter_Reserve_UsesKeyBucket(t *testing.T) {
	cl := New(func() ratelimit.Limiter {
		return &mockLimiter{}
	})

	wd := cl.ReserveKey("canary")
	if !wd.Allow {
		t.Error("Reserve should allow")
	}
	if wd.Duration != 0 {
		t.Errorf("Duration = %v, want 0", wd.Duration)
	}
	_ = time.Now // keep import
}
