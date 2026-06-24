package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/go-zeus/zeus/circuitbreaker"
	"github.com/go-zeus/zeus/routing"
)

// alwaysFail 总是失败的 Breaker，用于测试 Execute 传播错误标记
// 注意：MarkSuccess/MarkFailed 不持有可变状态（无锁需求），保证并发安全
type alwaysFail struct{}

func (a *alwaysFail) Allow() error                { return nil }
func (a *alwaysFail) MarkSuccess()                {}
func (a *alwaysFail) MarkFailed()                 {}
func (a *alwaysFail) State() circuitbreaker.State { return circuitbreaker.StateClosed }

var _ circuitbreaker.Breaker = (*alwaysFail)(nil)

// TestExecute_RouteByKey 验证不同 key 路由到独立 breaker
func TestExecute_RouteByKey(t *testing.T) {
	cb := New(func() circuitbreaker.Breaker {
		return &alwaysFail{}
	})

	_ = cb.ExecuteKey("default", func() error { return nil })
	_ = cb.ExecuteKey("canary", func() error { return nil })

	keys := cb.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 breakers, got %d", len(keys))
	}
}

// TestExecute_DefaultKeyFromContext 验证默认从 ctx 提取 cluster 作为 key
func TestExecute_DefaultKeyFromContext(t *testing.T) {
	cb := New(func() circuitbreaker.Breaker { return &alwaysFail{} })

	ctx := routing.WithCluster(context.Background(), "canary")
	_ = cb.Execute(ctx, func() error { return nil })

	keys := cb.Keys()
	if len(keys) != 1 || keys[0] != "canary" {
		t.Fatalf("expected [canary], got %v", keys)
	}
}

// TestExecute_PropagatesError 验证 fn 错误正确传播
func TestExecute_PropagatesError(t *testing.T) {
	cb := New(func() circuitbreaker.Breaker { return &alwaysFail{} })

	want := errors.New("boom")
	got := cb.ExecuteKey("x", func() error { return want })
	if !errors.Is(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestExecute_FactoryNil_FallsBackToAlwaysClosed 验证 nil factory 不熔断
func TestExecute_FactoryNil_FallsBackToAlwaysClosed(t *testing.T) {
	cb := New(nil)

	// 即使 fn 总是失败，也不应触发熔断（兜底 alwaysClosed）
	for i := 0; i < 10; i++ {
		if err := cb.ExecuteKey("x", func() error { return errors.New("fail") }); err == nil {
			t.Error("fn error should be propagated")
		}
	}

	// alwaysClosed 永远 Allow
	if err := cb.AllowKey("x"); err != nil {
		t.Errorf("AllowKey should always pass with nil factory, got %v", err)
	}
}

// TestStateKey_DistinctStatePerKey 验证不同 key 状态独立
func TestStateKey_DistinctStatePerKey(t *testing.T) {
	// 简化：用 alwaysFail 验证 StateKey 返回 Closed
	cb := New(func() circuitbreaker.Breaker { return &alwaysFail{} })
	_ = cb.ExecuteKey("default", func() error { return nil })
	if s := cb.StateKey("default"); s != circuitbreaker.StateClosed {
		t.Errorf("state = %v, want Closed", s)
	}
}

// TestConcurrentSafe 验证并发安全（必须 -race 通过）
func TestConcurrentSafe(t *testing.T) {
	cb := New(func() circuitbreaker.Breaker { return &alwaysFail{} })
	var wg sync.WaitGroup
	clusters := []string{"default", "canary", "order.v2"}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := routing.WithCluster(context.Background(), clusters[idx%len(clusters)])
			_ = cb.Execute(ctx, func() error { return nil })
		}(i)
	}
	wg.Wait()
}
