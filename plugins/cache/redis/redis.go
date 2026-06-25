// Package redis 提供基于 redis/go-redis 的 cache.Cache 实现。
//
// 设计目的：
//   - 让分布式缓存具备与内存缓存相同的 trace/metrics 自动集成
//   - 保持 cache.Cache 接口契约（Get/Set/Delete/Has/Close）
//   - 允许用户通过 Client(c) 拿到原生 *redis.Client 做高级操作
//
// 序列化策略：
//   - 仅接受 string / []byte 类型的 value（强制用户显式控制序列化格式）
//   - 复杂类型（struct/map/int/...）需用户自行 json.Marshal 后再 Set
//   - Get 返回的 string 包装为 any（适配 cache.Cache 接口的 (any, bool)）
//
// 不做的事：
//   - 不做 JSON / Gob 等隐式序列化（避免跨语言互通性陷阱）
//   - 不做 pub-sub / Lua 脚本（用原生 Client）
//   - 不做集群感知（依赖 *redis.Client 自身能力）
//
// 用法：
//
//	import (
//	    "github.com/redis/go-redis/v9"
//	    "github.com/go-zeus/zeus/cache"
//	    "github.com/go-zeus/zeus/plugins/cache/redis"
//	)
//
//	cli := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
//	c := redis.New(cli, redis.WithTracer(tracer), redis.WithMeter(meter))
//	defer c.Close()
//
//	_ = c.Set(ctx, "key", "value", cache.WithTTL(5*time.Minute))
//	v, ok := c.Get(ctx, "key") // v 是 string
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/metrics"
	mnoop "github.com/go-zeus/zeus/metrics/noop"
	"github.com/go-zeus/zeus/trace"
	tnoop "github.com/go-zeus/zeus/trace/noop"
)

const (
	metricCacheOpTotal    = "cache_op_total"
	metricCacheOpDuration = "cache_op_duration"
)

// ErrUnsupportedValueType Set 的 value 非 string/[]byte 时返回
//
// 用户应自行 json.Marshal 复杂类型后传入 []byte
var ErrUnsupportedValueType = errors.New("redis: unsupported value type, expect string or []byte")

// cacheImpl 包装 *redis.Client 实现 cache.Cache
type cacheImpl struct {
	client    *redis.Client
	tracer    trace.Tracer
	meter     metrics.Meter
	name      string
	recordKey bool // 是否在 span attrs 中记录 key（默认 false，避免敏感数据）

	managed bool // Close 是否关闭 client（默认 true；用户自管 client 时设为 false）
}

// Option 配置构造函数
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

// WithName 设置 metric label 中的 cache 标识（默认 "redis"）
//
// 多实例场景下用业务名区分（如 "session-cache" / "rate-limit-cache"）
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

// WithManagedClient 控制是否由本 Cache 管理 client 生命周期（默认 true）。
//
// 用户共享 client（如多个 Cache 实例用同一连接池）时设为 false，
// 此时 Close() 为 no-op，由用户自行 Close() client。
func WithManagedClient(managed bool) Option {
	return func(c *cacheImpl) {
		c.managed = managed
	}
}

// New 创建 Redis-backed Cache（实现 cache.Cache 接口）。
//
// 行为：
//   - 复用用户传入的 *redis.Client（不负责构造；用户可自由配置 PoolSize/DB/TLS 等）
//   - 自动注入 trace span（cache.get/set/delete/has）
//   - 自动上报 metrics（counter hit/miss + histogram 延迟）
//   - Close 时若 managed=true 则关闭 client
func New(client *redis.Client, opts ...Option) cache.Cache {
	c := &cacheImpl{
		client:  client,
		tracer:  tnoop.New(),
		meter:   mnoop.New(),
		name:    "redis",
		managed: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Get 按 key 取值；命中返回 (string, true)，未命中或过期返回 (nil, false)
func (c *cacheImpl) Get(ctx context.Context, key string) (any, bool) {
	_, span := c.startSpan(ctx, "cache.get", key)
	start := time.Now()

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		// redis.Nil 是未命中的标准错误（不视为系统错误）
		if errors.Is(err, redis.Nil) {
			c.recordMetric("get", "miss", time.Since(start))
			span.End()
			return nil, false
		}
		// 其他错误按 miss 计（cache 兜底；上层可读日志）
		c.recordMetric("get", "miss", time.Since(start))
		span.RecordError(err)
		span.End()
		return nil, false
	}
	c.recordMetric("get", "hit", time.Since(start))
	span.End()
	return val, true
}

// Set 写入 K-V；value 仅接受 string / []byte
//
// opts 可指定 TTL；不指定 TTL 视为永久
func (c *cacheImpl) Set(ctx context.Context, key string, val any, opts ...cache.Option) error {
	_, span := c.startSpan(ctx, "cache.set", key)
	start := time.Now()

	item := &cache.Item{Key: key, Value: val}
	for _, opt := range opts {
		if opt != nil {
			opt(item)
		}
	}

	s, err := toString(item.Value)
	if err != nil {
		c.recordMetric("set", "error", time.Since(start))
		span.RecordError(err)
		span.End()
		return err
	}

	if item.TTL > 0 {
		err = c.client.Set(ctx, key, s, item.TTL).Err()
	} else {
		// 0 TTL = 永久（Redis SET 无 TTL 参数时为永久）
		err = c.client.Set(ctx, key, s, 0).Err()
	}
	if err != nil {
		c.recordMetric("set", "error", time.Since(start))
		span.RecordError(err)
		span.End()
		return err
	}
	c.recordMetric("set", "ok", time.Since(start))
	span.End()
	return nil
}

// Delete 删除 key（不存在 no-op）
func (c *cacheImpl) Delete(ctx context.Context, key string) error {
	_, span := c.startSpan(ctx, "cache.delete", key)
	start := time.Now()

	// Del 接受多个 key，单个 key 用切片展开
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		c.recordMetric("delete", "error", time.Since(start))
		span.RecordError(err)
		span.End()
		return err
	}
	c.recordMetric("delete", "ok", time.Since(start))
	span.End()
	return nil
}

// Has 探测 key 是否存在（不读取值，性能优于 Get）
func (c *cacheImpl) Has(ctx context.Context, key string) bool {
	_, span := c.startSpan(ctx, "cache.has", key)
	defer span.End()
	start := time.Now()

	n, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		c.recordMetric("has", "error", time.Since(start))
		span.RecordError(err)
		return false
	}
	c.recordMetric("has", "ok", time.Since(start))
	return n > 0
}

// Close 关闭 client（managed=true 时）
func (c *cacheImpl) Close() error {
	if c.managed && c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Client 从 cache.Cache 实例中取回底层 *redis.Client
//
// 用于：Pipeline / Pub-Sub / Lua / SCAN 等高级操作
// 返回 (nil, false) 表示传入的不是本插件的实现
func Client(c cache.Cache) (*redis.Client, bool) {
	if impl, ok := c.(*cacheImpl); ok {
		return impl.client, true
	}
	return nil, false
}

// toString 把 cache value 转为 Redis 兼容的 string
//
// 仅接受 string / []byte；其他类型返回 ErrUnsupportedValueType
func toString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case []byte:
		return string(x), nil
	case nil:
		// 允许 nil 但语义为空字符串（避免 Set 后 Get 行为不一致）
		return "", nil
	default:
		return "", fmt.Errorf("%w: got %T", ErrUnsupportedValueType, v)
	}
}

// startSpan 创建带标准 attrs 的 span
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
// status: "hit"/"miss"（Get）；"ok"/"error"（Set/Delete/Has）
func (c *cacheImpl) recordMetric(op, status string, dur time.Duration) {
	labels := map[string]string{"cache": c.name, "op": op, "status": status}
	c.meter.Counter(metricCacheOpTotal, labels).Inc()
	c.meter.Histogram(metricCacheOpDuration, map[string]string{"cache": c.name, "op": op}).Observe(dur.Seconds())
}
