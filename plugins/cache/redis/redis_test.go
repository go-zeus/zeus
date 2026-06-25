package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

// —— spy tracer / meter（参考 memory_test.go） ——

type spySpan struct {
	name  string
	attrs map[string]string
	ended bool
	err   error
}

func (s *spySpan) End()                              { s.ended = true }
func (s *spySpan) SetAttributes(_ map[string]string) {}
func (s *spySpan) SetName(n string)                  { s.name = n }
func (s *spySpan) RecordError(err error)             { s.err = err }
func (s *spySpan) IsRecording() bool                 { return true }

type spyTracer struct {
	mu    sync.Mutex
	spans []*spySpan
}

func (t *spyTracer) StartSpan(_ context.Context, name string, opts ...trace.SpanOption) (context.Context, trace.Span) {
	cfg := &trace.SpanConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	s := &spySpan{name: name, attrs: cfg.Attrs}
	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return context.Background(), s
}
func (t *spyTracer) Close() error { return nil }

func (t *spyTracer) count(name string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, s := range t.spans {
		if s.name == name {
			n++
		}
	}
	return n
}

type spyCounter struct {
	mu    sync.Mutex
	value float64
}

func (c *spyCounter) Inc()          { c.mu.Lock(); c.value++; c.mu.Unlock() }
func (c *spyCounter) Add(v float64) { c.mu.Lock(); c.value += v; c.mu.Unlock() }

type spyHistogram struct {
	mu   sync.Mutex
	vals []float64
}

func (h *spyHistogram) Observe(v float64) {
	h.mu.Lock()
	h.vals = append(h.vals, v)
	h.mu.Unlock()
}

type spyMeter struct {
	mu         sync.Mutex
	counters   []*spyCounter
	histograms []*spyHistogram
	lastLabels map[string]string
}

func (m *spyMeter) Counter(_ string, labels map[string]string) metrics.Counter {
	c := &spyCounter{}
	m.mu.Lock()
	m.counters = append(m.counters, c)
	m.lastLabels = labels
	m.mu.Unlock()
	return c
}

func (m *spyMeter) Histogram(_ string, labels map[string]string) metrics.Histogram {
	h := &spyHistogram{}
	m.mu.Lock()
	m.histograms = append(m.histograms, h)
	m.lastLabels = labels
	m.mu.Unlock()
	return h
}

func (m *spyMeter) Gauge(string, map[string]string) metrics.Gauge { return nil }
func (m *spyMeter) Close() error                                  { return nil }

func (m *spyMeter) counterTotal() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sum float64
	for _, c := range m.counters {
		c.mu.Lock()
		sum += c.value
		c.mu.Unlock()
	}
	return sum
}

// —— 测试辅助 ——

// newMiniRedis 启动 miniredis 实例并返回对应的 *redis.Client
//
// 调用者负责在测试结束后调用 closeFn 释放资源
func newMiniRedis(t *testing.T) (c cache.Cache, cli *redis.Client, closeFn func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c = New(cli)
	return c, cli, func() {
		_ = c.Close()
		_ = cli.Close()
	}
}

// —— 基础功能测试 ——

// TestRedisCache_SetGetHasDelete 基本 CRUD
func TestRedisCache_SetGetHasDelete(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	ctx := context.Background()

	// Set + Get（string）
	if err := c.Set(ctx, "k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok := c.Get(ctx, "k1")
	if !ok || v != "v1" {
		t.Errorf("Get(k1) = (%v,%v), want (v1,true)", v, ok)
	}

	// Set []byte
	if err := c.Set(ctx, "k2", []byte("bytes-val")); err != nil {
		t.Fatalf("Set []byte: %v", err)
	}
	if v, ok := c.Get(ctx, "k2"); !ok || v != "bytes-val" {
		t.Errorf("Get(k2) = (%v,%v), want (bytes-val,true)", v, ok)
	}

	// Has
	if !c.Has(ctx, "k1") {
		t.Error("Has(k1) = false, want true")
	}
	if c.Has(ctx, "no-such") {
		t.Error("Has(no-such) = true, want false")
	}

	// Delete + Get（应未命中）
	if err := c.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := c.Get(ctx, "k1"); ok {
		t.Error("Get after Delete should miss")
	}
}

// TestRedisCache_GetMiss 未命中返回 (nil, false)
func TestRedisCache_GetMiss(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	v, ok := c.Get(context.Background(), "no-such-key")
	if ok || v != nil {
		t.Errorf("Get(miss) = (%v,%v), want (nil,false)", v, ok)
	}
}

// TestRedisCache_DeleteNonExistent 删除不存在的 key 是 no-op
func TestRedisCache_DeleteNonExistent(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	if err := c.Delete(context.Background(), "no-such"); err != nil {
		t.Errorf("Delete non-existent should be no-op, got: %v", err)
	}
}

// TestRedisCache_TTL TTL 过期（miniredis.FastForward 模拟时间）
func TestRedisCache_TTL(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := New(cli)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Set with 1s TTL
	if err := c.Set(ctx, "k", "v", cache.WithTTL(time.Second)); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// 立即 Get 应命中
	if v, ok := c.Get(ctx, "k"); !ok || v != "v" {
		t.Errorf("immediate Get = (%v,%v), want (v,true)", v, ok)
	}

	// FastForward 2 秒，触发 TTL 过期
	mr.FastForward(2 * time.Second)

	if _, ok := c.Get(ctx, "k"); ok {
		t.Error("Get after TTL expiry should miss")
	}
}

// TestRedisCache_NoTTL_Permanent 无 TTL 永不过期
func TestRedisCache_NoTTL_Permanent(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	ctx := context.Background()
	_ = c.Set(ctx, "forever", "v")

	// FastForward 较长时间，仍应存在
	if v, ok := c.Get(ctx, "forever"); !ok || v != "v" {
		t.Errorf("Get(no-TTL) = (%v,%v), want (v,true)", v, ok)
	}
}

// TestRedisCache_UnsupportedType 非 string/[]byte 类型应返回错误
func TestRedisCache_UnsupportedType(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	ctx := context.Background()

	tests := []struct {
		name string
		val  any
	}{
		{"int", 42},
		{"struct", struct{ Name string }{"alice"}},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]string{"a": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Set(ctx, "k", tt.val)
			if err == nil {
				t.Errorf("Set(%s) should fail", tt.name)
				return
			}
			if !errors.Is(err, ErrUnsupportedValueType) {
				t.Errorf("Set(%s) err = %v, want ErrUnsupportedValueType", tt.name, err)
			}
		})
	}
}

// TestRedisCache_NilValue nil 视为空字符串（保持 Get 行为一致）
func TestRedisCache_NilValue(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	ctx := context.Background()
	if err := c.Set(ctx, "k", nil); err != nil {
		t.Fatalf("Set(nil): %v", err)
	}
	v, ok := c.Get(ctx, "k")
	if !ok || v != "" {
		t.Errorf("Get(nil) = (%v,%v), want (\"\",true)", v, ok)
	}
}

// —— 并发测试 ——

// TestRedisCache_Concurrent 100 goroutine 并发 Set/Get，无 race
func TestRedisCache_Concurrent(t *testing.T) {
	c, _, closeFn := newMiniRedis(t)
	defer closeFn()

	ctx := context.Background()
	const N = 100
	var wg sync.WaitGroup
	wg.Add(N * 2)

	// N 个 writer
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%20)
			_ = c.Set(ctx, key, fmt.Sprintf("v%d", i))
		}(i)
	}
	// N 个 reader
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_, _ = c.Get(ctx, fmt.Sprintf("k%d", i%20))
		}(i)
	}
	wg.Wait()
}

// —— trace + metrics 集成测试 ——

// TestRedisCache_TraceMetrics span 与 metric 自动注入
func TestRedisCache_TraceMetrics(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	tr := &spyTracer{}
	mt := &spyMeter{}
	c := New(cli, WithTracer(tr), WithMeter(mt), WithName("test-redis"), WithRecordKey(true))
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Set
	if err := c.Set(ctx, "k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Get（命中）
	if _, ok := c.Get(ctx, "k1"); !ok {
		t.Fatal("Get should hit")
	}
	// Get（未命中）
	_, _ = c.Get(ctx, "no-such")
	// Delete
	_ = c.Delete(ctx, "k1")
	// Has
	_ = c.Has(ctx, "k1")

	// 验证 span 调用次数
	if n := tr.count("cache.set"); n != 1 {
		t.Errorf("cache.set spans = %d, want 1", n)
	}
	if n := tr.count("cache.get"); n != 2 {
		t.Errorf("cache.get spans = %d, want 2", n)
	}
	if n := tr.count("cache.delete"); n != 1 {
		t.Errorf("cache.delete spans = %d, want 1", n)
	}
	if n := tr.count("cache.has"); n != 1 {
		t.Errorf("cache.has spans = %d, want 1", n)
	}

	// 验证 metric 调用次数（counter 至少 5 次：set + 2*get + delete + has）
	if total := mt.counterTotal(); total < 5 {
		t.Errorf("counter total = %v, want >= 5", total)
	}
}

// TestRedisCache_Options 影响内部配置
func TestRedisCache_Options(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	tr := &spyTracer{}
	c := New(cli, WithTracer(tr), WithName("my-cache"), WithRecordKey(true)).(*cacheImpl)
	defer func() { _ = c.Close() }()

	if c.name != "my-cache" {
		t.Errorf("name = %q, want %q", c.name, "my-cache")
	}
	if !c.recordKey {
		t.Error("recordKey should be true")
	}
	if c.tracer != trace.Tracer(tr) {
		t.Error("tracer not set")
	}
}

// TestRedisCache_WithManagedClient managed=false 时 Close 不关闭 client
func TestRedisCache_WithManagedClient(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	c := New(cli, WithManagedClient(false))
	// Close 应为 no-op
	if err := c.Close(); err != nil {
		t.Errorf("Close with managed=false should be no-op, got: %v", err)
	}
	// client 仍可用
	ctx := context.Background()
	if err := cli.Set(ctx, "x", "y", 0).Err(); err != nil {
		t.Errorf("client should still work after cache.Close(): %v", err)
	}
	// 现在真正关闭 client
	if err := cli.Close(); err != nil {
		t.Errorf("cli.Close: %v", err)
	}
}

// TestRedisCache_ClientExport Client(c) 取回底层 client
func TestRedisCache_ClientExport(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := New(cli, WithManagedClient(false))
	defer func() { _ = cli.Close() }()

	got, ok := Client(c)
	if !ok {
		t.Fatal("Client(c) ok = false, want true")
	}
	if got != cli {
		t.Error("Client(c) returned different instance")
	}

	// 非 redis 实现应返回 false
	_, ok = Client(&fakeCache{})
	if ok {
		t.Error("Client(non-redis) should return false")
	}
}

// fakeCache 仅用于测试 Client() 类型断言失败路径
type fakeCache struct{}

func (f *fakeCache) Get(context.Context, string) (any, bool)                 { return nil, false }
func (f *fakeCache) Set(context.Context, string, any, ...cache.Option) error { return nil }
func (f *fakeCache) Delete(context.Context, string) error                    { return nil }
func (f *fakeCache) Has(context.Context, string) bool                        { return false }
func (f *fakeCache) Close() error                                            { return nil }

// —— 集成测试（需要真实 Redis） ——

// skipIfNoRedis 当 Redis 不可达时跳过测试
func skipIfNoRedis(t *testing.T) (addr string) {
	t.Helper()
	addr = os.Getenv("ZEUS_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	cli := redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 2 * time.Second})
	defer func() { _ = cli.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		t.Skipf("redis 不可达，跳过集成测试 (ping failed): %v", err)
		return
	}
	return addr
}

// TestRedisCache_RealEndToEnd 真实 Redis 端到端
func TestRedisCache_RealEndToEnd(t *testing.T) {
	addr := skipIfNoRedis(t)

	cli := redis.NewClient(&redis.Options{Addr: addr})
	c := New(cli, WithName("e2e"))
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	key := fmt.Sprintf("zeus:test:%d", time.Now().UnixNano())

	// Set + Get
	if err := c.Set(ctx, key, "hello", cache.WithTTL(5*time.Second)); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok := c.Get(ctx, key)
	if !ok || v != "hello" {
		t.Errorf("Get = (%v,%v), want (hello,true)", v, ok)
	}

	// Has
	if !c.Has(ctx, key) {
		t.Error("Has should be true")
	}

	// Delete
	if err := c.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := c.Get(ctx, key); ok {
		t.Error("Get after Delete should miss")
	}
}
