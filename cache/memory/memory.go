// Package memory 提供基于 sync.Map 的进程内缓存实现。
//
// 设计目的：
//   - 零依赖（仅用标准库）
//   - 用作单进程内存缓存（小规模数据热缓存）
//   - 用作单元测试的 mock cache
//
// 行为：
//   - TTL 双路径清理：
//     1) 懒清理：Get 时检查过期，过期则删除并返回 false
//     2) 后台清理：周期扫描（默认 60s），主动删除过期项
//   - 并发安全（sync.Map）
//   - Close 停止后台 goroutine（防泄漏）
//
// 局限：
//   - 不持久化（重启后丢失）
//   - 不支持跨进程（多实例间不感知）
//   - 不支持淘汰策略（LRU/LFU）—— 用 plugins/cache/redis
//   - 不做内存上限管理（OOM 风险由调用方控制）
//
// 与 trace/metrics 集成：
//   - 每次操作自动创建 span（attrs: cache_key(可配), cache）
//   - 每次操作自动上报 metrics（counter hit/miss, histogram latency）
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/metrics"
	mnoop "github.com/go-zeus/zeus/metrics/noop"
	"github.com/go-zeus/zeus/trace"
	tnoop "github.com/go-zeus/zeus/trace/noop"
)

const (
	// defaultCleanupInterval 后台清理周期
	defaultCleanupInterval = 60 * time.Second

	metricCacheOpTotal    = "cache_op_total"
	metricCacheOpDuration = "cache_op_duration"
)

// entry 单个缓存项
type entry struct {
	value    any
	expireAt time.Time // 零值表示永不过期
}

// expired 是否已过期
func (e *entry) expired(now time.Time) bool {
	if e.expireAt.IsZero() {
		return false
	}
	return now.After(e.expireAt)
}

// cacheImpl 内存缓存实现
type cacheImpl struct {
	data      sync.Map // map[string]*entry
	tracer    trace.Tracer
	meter     metrics.Meter
	name      string
	recordKey bool // 是否在 span attrs 中记录 key（默认 false，避免敏感数据）

	cleanupInterval time.Duration // 后台清理周期
	stop            chan struct{} // 通知后台 goroutine 退出
	done            chan struct{} // cleaner goroutine 退出后关闭（用于测试同步）
	once            sync.Once
}

// Option 配置 cache
type Option func(*cacheImpl)

// WithTracer 注入 Tracer（默认 noop）
func WithTracer(t trace.Tracer) Option {
	return func(c *cacheImpl) {
		if t != nil {
			c.tracer = t
		}
	}
}

// WithMeter 注入 Meter（默认 noop）
func WithMeter(m metrics.Meter) Option {
	return func(c *cacheImpl) {
		if m != nil {
			c.meter = m
		}
	}
}

// WithName 设置 metric label 中的 cache 标识（默认 "memory"）
func WithName(name string) Option {
	return func(c *cacheImpl) {
		if name != "" {
			c.name = name
		}
	}
}

// WithRecordKey 设置是否在 span attrs 中记录 key
//
// 默认关闭以避免敏感数据进入 trace；显式开启用于调试
func WithRecordKey(b bool) Option {
	return func(c *cacheImpl) {
		c.recordKey = b
	}
}

// WithCleanupInterval 设置后台清理周期（默认 60s）
//
// d <= 0 时禁用后台清理（仅依赖懒清理）
//
// 注意：本 Option 直接赋值（不限 > 0），以便用户显式禁用 cleaner goroutine
func WithCleanupInterval(d time.Duration) Option {
	return func(c *cacheImpl) {
		c.cleanupInterval = d
	}
}

// New 创建内存缓存
func New(opts ...Option) cache.Cache {
	c := &cacheImpl{
		tracer:          tnoop.New(),
		meter:           mnoop.New(),
		name:            "memory",
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
		cleanupInterval: defaultCleanupInterval,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	c.startCleaner()
	return c
}

// Get 按 key 取值；命中返回 (value, true)，未命中或过期返回 (nil, false)
func (c *cacheImpl) Get(ctx context.Context, key string) (any, bool) {
	_, span := c.startSpan(ctx, "cache.get", key)
	start := time.Now()

	v, ok := c.data.Load(key)
	if !ok {
		c.recordMetric("get", "miss", time.Since(start))
		span.End()
		return nil, false
	}
	e := v.(*entry)
	if e.expired(time.Now()) {
		c.data.Delete(key) // 懒清理
		c.recordMetric("get", "miss", time.Since(start))
		span.End()
		return nil, false
	}
	c.recordMetric("get", "hit", time.Since(start))
	span.End()
	return e.value, true
}

// Set 写入 K-V；opts 可指定 TTL
func (c *cacheImpl) Set(ctx context.Context, key string, val any, opts ...cache.Option) error {
	_, span := c.startSpan(ctx, "cache.set", key)
	start := time.Now()

	item := &cache.Item{Key: key, Value: val}
	for _, opt := range opts {
		if opt != nil {
			opt(item)
		}
	}

	e := &entry{value: item.Value}
	if item.TTL > 0 {
		e.expireAt = time.Now().Add(item.TTL)
	}
	c.data.Store(key, e)

	c.recordMetric("set", "ok", time.Since(start))
	span.End()
	return nil
}

// Delete 删除 key（不存在 no-op）
func (c *cacheImpl) Delete(ctx context.Context, key string) error {
	_, span := c.startSpan(ctx, "cache.delete", key)
	start := time.Now()

	c.data.Delete(key)

	c.recordMetric("delete", "ok", time.Since(start))
	span.End()
	return nil
}

// Has 探测 key 是否存在（不触发懒清理）
//
// 注意：可能返回 true 但随后的 Get 返回 false（因后台清理在中间发生）
func (c *cacheImpl) Has(ctx context.Context, key string) bool {
	_, span := c.startSpan(ctx, "cache.has", key)
	defer span.End()

	v, ok := c.data.Load(key)
	if !ok {
		return false
	}
	e := v.(*entry)
	// Has 不删除过期项（避免 Get 路径外的写操作），仅返回 false
	return !e.expired(time.Now())
}

// Close 停止后台 goroutine
func (c *cacheImpl) Close() error {
	c.once.Do(func() {
		close(c.stop)
	})
	return nil
}

// startCleaner 启动后台 goroutine 周期清理过期项
//
// 无论是否启动 goroutine，都会在合适时机关闭 done channel：
//   - 不启动（cleanupInterval<=0）：立即关闭，标识 cleaner "从未运行"
//   - 启动：goroutine 退出时 defer close(done)
//
// done channel 用于测试同步（等待 cleaner 退出），避免依赖 runtime.NumGoroutine 的不稳定数值
func (c *cacheImpl) startCleaner() {
	if c.cleanupInterval <= 0 {
		close(c.done)
		return
	}
	go func() {
		defer close(c.done)
		ticker := time.NewTicker(c.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-ticker.C:
				c.cleanupExpired()
			}
		}
	}()
}

// cleanupExpired 扫描所有项，删除过期项
func (c *cacheImpl) cleanupExpired() {
	now := time.Now()
	c.data.Range(func(key, value any) bool {
		e := value.(*entry)
		if e.expired(now) {
			c.data.Delete(key)
		}
		return true
	})
}

// startSpan 创建带 attrs 的 span
func (c *cacheImpl) startSpan(ctx context.Context, name, key string) (context.Context, trace.Span) {
	attrs := map[string]string{"cache": c.name}
	if c.recordKey {
		attrs["cache_key"] = key
	}
	opt := func(cfg *trace.SpanConfig) { cfg.Attrs = attrs }
	return c.tracer.StartSpan(ctx, name, opt)
}

// recordMetric 上报 op 计数 + 延迟
//
// status: "hit"/"miss"（Get）；"ok"（Set/Delete/Has）
func (c *cacheImpl) recordMetric(op, status string, dur time.Duration) {
	labels := map[string]string{"cache": c.name, "op": op, "status": status}
	c.meter.Counter(metricCacheOpTotal, labels).Inc()
	c.meter.Histogram(metricCacheOpDuration, map[string]string{"cache": c.name, "op": op}).Observe(dur.Seconds())
}
