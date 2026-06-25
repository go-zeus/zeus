package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/retry"
	"github.com/go-zeus/zeus/routing"
)

// mockRetrier 固定重试 N 次
type mockRetrier struct {
	maxCount int
	count    int
}

func (m *mockRetrier) Next() (time.Duration, bool) {
	if m.count >= m.maxCount {
		return 0, false
	}
	m.count++
	return time.Duration(m.count) * 10 * time.Millisecond, true
}
func (m *mockRetrier) Reset()     { m.count = 0 }
func (m *mockRetrier) Count() int { return m.count }

var _ retry.Retrier = (*mockRetrier)(nil)

// TestNewRetriever_DefaultFactory 验证默认 factory 用于未匹配 key
func TestNewRetriever_DefaultFactory(t *testing.T) {
	cr := New(func() retry.Retrier { return &mockRetrier{maxCount: 3} })

	r := cr.NewRetrieverForKey("anything")
	got := 0
	for {
		_, ok := r.Next()
		if !ok {
			break
		}
		got++
	}
	if got != 3 {
		t.Errorf("retries = %d, want 3", got)
	}
}

// TestNewRetriever_KeyOverride 验证 Set 后指定 key 用专用 factory
func TestNewRetriever_KeyOverride(t *testing.T) {
	cr := New(func() retry.Retrier { return &mockRetrier{maxCount: 1} })
	cr.Set("canary", func() retry.Retrier { return &mockRetrier{maxCount: 5} })

	// default factory
	dr := cr.NewRetrieverForKey("default")
	cnt := 0
	for {
		_, ok := dr.Next()
		if !ok {
			break
		}
		cnt++
	}
	if cnt != 1 {
		t.Errorf("default retries = %d, want 1", cnt)
	}

	// canary factory
	cr2 := cr.NewRetrieverForKey("canary")
	cnt2 := 0
	for {
		_, ok := cr2.Next()
		if !ok {
			break
		}
		cnt2++
	}
	if cnt2 != 5 {
		t.Errorf("canary retries = %d, want 5", cnt2)
	}
}

// TestNewRetriever_DefaultKeyFromContext 验证从 ctx 提取 cluster
func TestNewRetriever_DefaultKeyFromContext(t *testing.T) {
	called := false
	cr := New(func() retry.Retrier {
		called = true
		return &mockRetrier{maxCount: 1}
	})

	ctx := routing.WithCluster(context.Background(), "canary")
	_ = cr.NewRetriever(ctx)

	if !called {
		t.Error("factory should be called")
	}
}

// TestNewRetriever_FactoryNil_FallsBackToNoRetry 验证 nil factory 立即停止
func TestNewRetriever_FactoryNil_FallsBackToNoRetry(t *testing.T) {
	cr := New(nil)
	r := cr.NewRetrieverForKey("x")
	_, ok := r.Next()
	if ok {
		t.Error("nil factory should produce no-retry")
	}
}

// TestConcurrentSafe 验证并发安全
func TestConcurrentSafe(t *testing.T) {
	cr := New(func() retry.Retrier { return &mockRetrier{maxCount: 1} })
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := routing.WithCluster(context.Background(), "canary")
			r := cr.NewRetriever(ctx)
			r.Next()
		}(i)
	}
	wg.Wait()
}
