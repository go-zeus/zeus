package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

// —— spy tracer / meter 复用 database/sql 测试中的设计 ——

type spySpan struct {
	name  string
	attrs map[string]string
	ended bool
}

func (s *spySpan) End()                              { s.ended = true }
func (s *spySpan) SetAttributes(_ map[string]string) {}
func (s *spySpan) SetName(n string)                  { s.name = n }
func (s *spySpan) RecordError(_ error)               {}
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

func (t *spyTracer) first(name string) *spySpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.spans {
		if s.name == name {
			return s
		}
	}
	return nil
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
	lastLabels map[string]string // 最后一次 Counter/Histogram 调用的 labels
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

// —— 基础功能测试 ——

// TestSet_Get_Has_Delete 基本 CRUD
func TestSet_Get_Has_Delete(t *testing.T) {
	c := New()
	defer c.Close()

	ctx := context.Background()

	// Set + Get
	if err := c.Set(ctx, "k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok := c.Get(ctx, "k1")
	if !ok || v != "v1" {
		t.Errorf("Get(k1) = (%v,%v), want (v1,true)", v, ok)
	}

	// Has
	if !c.Has(ctx, "k1") {
		t.Error("Has(k1) = false, want true")
	}

	// Delete + Get（应未命中）
	if err := c.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := c.Get(ctx, "k1"); ok {
		t.Error("Get after Delete should miss")
	}
}

// TestGet_Miss 未命中返回 (nil, false)
func TestGet_Miss(t *testing.T) {
	c := New()
	defer c.Close()
	v, ok := c.Get(context.Background(), "no-such-key")
	if ok || v != nil {
		t.Errorf("Get(miss) = (%v,%v), want (nil,false)", v, ok)
	}
}

// TestDelete_NonExistent 删除不存在的 key 是 no-op
func TestDelete_NonExistent(t *testing.T) {
	c := New()
	defer c.Close()
	if err := c.Delete(context.Background(), "no-such"); err != nil {
		t.Errorf("Delete non-existent should be no-op, got: %v", err)
	}
}

// TestSet_TTL_Expiry TTL 过期（懒清理路径）
func TestSet_TTL_Expiry(t *testing.T) {
	c := New(WithCleanupInterval(0)) // 禁用后台清理，只测懒清理
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k", "v", cache.WithTTL(50*time.Millisecond))

	// 立即 Get 应命中
	if v, ok := c.Get(ctx, "k"); !ok || v != "v" {
		t.Errorf("immediate Get = (%v,%v), want (v,true)", v, ok)
	}

	// 等 TTL 过期
	time.Sleep(80 * time.Millisecond)

	// Get 应未命中（懒清理删除）
	if _, ok := c.Get(ctx, "k"); ok {
		t.Error("Get after TTL expiry should miss")
	}
}

// TestSet_NoTTL_NoExpiry 无 TTL 永不过期
func TestSet_NoTTL_NoExpiry(t *testing.T) {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "forever", "v")
	time.Sleep(30 * time.Millisecond)
	if v, ok := c.Get(ctx, "forever"); !ok || v != "v" {
		t.Errorf("Get(no-TTL) = (%v,%v), want (v,true)", v, ok)
	}
}

// TestSet_TTL_ZeroOrNegative TTL<=0 视为永久
func TestSet_TTL_ZeroOrNegative(t *testing.T) {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k1", "v1", cache.WithTTL(0))
	_ = c.Set(ctx, "k2", "v2", cache.WithTTL(-time.Second))

	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get(ctx, "k1"); !ok {
		t.Error("TTL=0 should be permanent")
	}
	if _, ok := c.Get(ctx, "k2"); !ok {
		t.Error("TTL<0 should be permanent")
	}
}

// TestBackgroundCleanup 后台清理路径
func TestBackgroundCleanup(t *testing.T) {
	c := New(WithCleanupInterval(50 * time.Millisecond)) // 50ms 扫描一次
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k", "v", cache.WithTTL(30*time.Millisecond))

	// 等 TTL 过期 + 等后台扫描
	time.Sleep(150 * time.Millisecond)

	// 后台清理后，Has 应返回 false（不需要 Get 触发）
	if c.Has(ctx, "k") {
		t.Error("Has after background cleanup should be false")
	}
}

// TestHas_DoesNotDelete Has 不删除过期项（避免 Get 路径外的写）
func TestHas_DoesNotDelete(t *testing.T) {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k", "v", cache.WithTTL(20*time.Millisecond))

	time.Sleep(40 * time.Millisecond)

	// Has 返回 false 但不删除
	if c.Has(ctx, "k") {
		t.Error("Has(expired) should return false")
	}
	// sync.Map 中仍存在（懒删除未触发）
	count := 0
	c.(*cacheImpl).data.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("sync.Map count after Has(expired) = %d, want 1 (Has should not delete)", count)
	}
}

// —— 并发测试 ——

// TestConcurrent_SetGet 1000 goroutine 并发 Set/Get，无 race
func TestConcurrent_SetGet(t *testing.T) {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	const N = 1000
	var wg sync.WaitGroup
	wg.Add(N * 2)

	// N 个 writer
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_ = c.Set(ctx, fmt.Sprintf("k%d", i%100), i)
		}(i)
	}
	// N 个 reader
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_, _ = c.Get(ctx, fmt.Sprintf("k%d", i%100))
		}(i)
	}
	wg.Wait()
}

// TestConcurrent_Counts 并发下计数正确
func TestConcurrent_Counts(t *testing.T) {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	const N = 500
	var hits int64
	var wg sync.WaitGroup
	wg.Add(N)

	// 全部并发 Set 同一 key
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_ = c.Set(ctx, "shared", "v")
		}()
	}
	wg.Wait()

	// 并发 Get
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if _, ok := c.Get(ctx, "shared"); ok {
				atomic.AddInt64(&hits, 1)
			}
		}()
	}
	wg.Wait()

	if hits != N {
		t.Errorf("hits = %d, want %d", hits, N)
	}
}

// —— goroutine 泄漏测试 ——

// TestClose_StopsCleaner Close 后 cleaner goroutine 退出
//
// 判定策略：通过 done channel 等待退出信号（替代 runtime.NumGoroutine 数值比较）
// 原因：NumGoroutine 数值会被其他并行测试的 cleaner goroutine 干扰，导致 flaky
func TestClose_StopsCleaner(t *testing.T) {
	c := New(WithCleanupInterval(10 * time.Millisecond))
	impl := c.(*cacheImpl)

	// 等待 cleaner goroutine 启动
	time.Sleep(20 * time.Millisecond)

	// done 此时应该未关闭（cleaner 还在运行）
	select {
	case <-impl.done:
		t.Error("done should not be closed while cleaner is running")
	default:
		// ok
	}

	_ = c.Close()

	// done 应在合理时间内关闭（cleaner goroutine 已退出）
	select {
	case <-impl.done:
		// ok: cleaner goroutine 已退出
	case <-time.After(time.Second):
		t.Error("cleaner goroutine did not stop after Close (done channel not closed)")
	}
}

// TestClose_StopsCleaner_Disabled 禁用 cleaner 时 done channel 仍然关闭
func TestClose_StopsCleaner_Disabled(t *testing.T) {
	c := New(WithCleanupInterval(0))
	impl := c.(*cacheImpl)

	// cleanupInterval<=0 时 startCleaner 立即关闭 done
	select {
	case <-impl.done:
		// ok
	default:
		t.Error("done should be closed immediately when cleaner is disabled")
	}

	_ = c.Close()
}

// TestClose_Idempotent Close 多次调用安全
func TestClose_Idempotent(t *testing.T) {
	c := New()
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close should be no-op, got: %v", err)
	}
}

// —— trace / metrics 集成测试 ——

// TestSetGet_CreatesSpans span 自动创建 + attrs 包含 cache name
func TestSetGet_CreatesSpans(t *testing.T) {
	tracer := &spyTracer{}
	c := New(WithTracer(tracer), WithName("user-cache"))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k", "v")
	_, _ = c.Get(ctx, "k")

	if got := tracer.count("cache.set"); got != 1 {
		t.Errorf("cache.set span count = %d, want 1", got)
	}
	if got := tracer.count("cache.get"); got != 1 {
		t.Errorf("cache.get span count = %d, want 1", got)
	}

	span := tracer.first("cache.get")
	if span.attrs["cache"] != "user-cache" {
		t.Errorf("attrs[cache] = %q, want user-cache", span.attrs["cache"])
	}
}

// TestWithRecordKey 开启后在 span attrs 中记录 key
func TestWithRecordKey(t *testing.T) {
	tracer := &spyTracer{}
	c := New(WithTracer(tracer), WithRecordKey(true))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "my-sensitive-key", "v")

	span := tracer.first("cache.set")
	if span.attrs["cache_key"] != "my-sensitive-key" {
		t.Errorf("attrs[cache_key] = %q, want my-sensitive-key", span.attrs["cache_key"])
	}
}

// TestRecordKey_OffByDefault 默认不记录 key
func TestRecordKey_OffByDefault(t *testing.T) {
	tracer := &spyTracer{}
	c := New(WithTracer(tracer))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "any-key", "v")

	span := tracer.first("cache.set")
	if _, exists := span.attrs["cache_key"]; exists {
		t.Error("cache_key should not be in attrs by default")
	}
}

// TestMetrics_HitMiss Get 命中/未命中分别上报 hit/miss status
func TestMetrics_HitMiss(t *testing.T) {
	meter := &spyMeter{}
	c := New(WithMeter(meter), WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "hit-key", "v")

	// 命中
	_, _ = c.Get(ctx, "hit-key")
	// 未命中
	_, _ = c.Get(ctx, "miss-key")

	// counter 总数 = Set(1) + Get×2 = 3
	if got := meter.counterTotal(); got != 3 {
		t.Errorf("counter total = %v, want 3", got)
	}

	// histogram 数 = Set(1) + Get×2 = 3
	meter.mu.Lock()
	hCount := len(meter.histograms)
	meter.mu.Unlock()
	if hCount != 3 {
		t.Errorf("histogram count = %d, want 3", hCount)
	}
}

// TestEndToEnd_MetricsAndTrace 集成验证：Set+Get+Delete 触发正确数量的 span 和 metric
func TestEndToEnd_MetricsAndTrace(t *testing.T) {
	tracer := &spyTracer{}
	meter := &spyMeter{}
	c := New(WithTracer(tracer), WithMeter(meter), WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k", "v")
	_, _ = c.Get(ctx, "k")
	_ = c.Delete(ctx, "k")

	// 验证 span 数量（每种操作 1 个）
	if got := tracer.count("cache.set"); got != 1 {
		t.Errorf("cache.set span = %d, want 1", got)
	}
	if got := tracer.count("cache.get"); got != 1 {
		t.Errorf("cache.get span = %d, want 1", got)
	}
	if got := tracer.count("cache.delete"); got != 1 {
		t.Errorf("cache.delete span = %d, want 1", got)
	}

	// 验证 metric 数量
	if got := meter.counterTotal(); got != 3 {
		t.Errorf("counter total = %v, want 3", got)
	}
}

// TestSet_VariousValueTypes 支持多种 value 类型
func TestSet_VariousValueTypes(t *testing.T) {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	cases := []struct {
		key string
		val any
	}{
		{"str", "hello"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"nil", nil},
		{"struct", struct{ Name string }{"alice"}},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]int{"a": 1}},
	}

	for _, tc := range cases {
		if err := c.Set(ctx, tc.key, tc.val); err != nil {
			t.Errorf("Set(%s, %v): %v", tc.key, tc.val, err)
			continue
		}
		v, ok := c.Get(ctx, tc.key)
		if !ok {
			t.Errorf("Get(%s) miss", tc.key)
			continue
		}
		// 注意：比较复合类型用 fmt.Sprint 避免类型不匹配
		if fmt.Sprint(v) != fmt.Sprint(tc.val) {
			t.Errorf("Get(%s) = %v, want %v", tc.key, v, tc.val)
		}
	}
}

// ExampleNew 简单使用示例
func ExampleNew() {
	c := New(WithCleanupInterval(0))
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "greeting", "hello", cache.WithTTL(time.Minute))
	v, _ := c.Get(ctx, "greeting")
	fmt.Println(v)
	// Output: hello
}
