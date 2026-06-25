package components

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/cache"
)

// mockCache 测试用 mock，仅记录 Close 调用
type mockCache struct {
	mu          sync.Mutex
	closeCalled bool
}

func (m *mockCache) Get(context.Context, string) (any, bool)                 { return nil, false }
func (m *mockCache) Set(context.Context, string, any, ...cache.Option) error { return nil }
func (m *mockCache) Delete(context.Context, string) error                    { return nil }
func (m *mockCache) Has(context.Context, string) bool                        { return false }
func (m *mockCache) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

// TestCacheComponent_Lifecycle OnStop 调 Close
func TestCacheComponent_Lifecycle(t *testing.T) {
	mock := &mockCache{}
	cc := NewCacheComponent(mock)

	c := NewContainer()
	_ = c.Register(cc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	mock.mu.Lock()
	if !mock.closeCalled {
		t.Error("Close should be called on Stop")
	}
	mock.mu.Unlock()
}

// TestCacheComponent_NilCache cache 为 nil 时 no-op
func TestCacheComponent_NilCache(t *testing.T) {
	cc := NewCacheComponent(nil)

	c := NewContainer()
	_ = c.Register(cc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Errorf("Start with nil cache should be no-op, got: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop with nil cache should be no-op, got: %v", err)
	}
}

// TestCacheComponent_Provide Provide 发布 cache.Cache
func TestCacheComponent_Provide(t *testing.T) {
	mock := &mockCache{}
	cc := NewCacheComponent(mock)

	c := NewContainer()
	_ = c.Register(cc)

	got := make(chan cache.Cache, 1)
	_ = c.Register(&cacheCapturer{capture: got})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case v := <-got:
		if v == nil {
			t.Error("captured cache is nil")
		}
	case <-time.After(time.Second):
		t.Fatal("cache not captured within 1s")
	}
}

// cacheCapturer 测试辅助
type cacheCapturer struct {
	capture chan<- cache.Cache
}

func (d *cacheCapturer) Name() string      { return "cache_capturer" }
func (d *cacheCapturer) Depends() []string { return []string{"cache"} }
func (d *cacheCapturer) Provide(_ Context) (any, error) {
	return d, nil
}
func (d *cacheCapturer) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			c, err := Type[cache.Cache](ctx)
			if err != nil {
				return err
			}
			d.capture <- c
			return nil
		},
	}
}
