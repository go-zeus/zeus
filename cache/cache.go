// Package cache 提供缓存（KV 存储）的统一抽象。
//
// 设计动机：
//   - 让缓存操作像 HTTP handler 一样获得 trace / metrics 自动集成
//   - 主包零依赖（内置实现用 sync.Map + 时间戳过期）
//   - 第三方实现（Redis/Memcached/etcd）放 plugins/cache/<vendor>
//
// 抽象边界：
//   - 仅暴露 Get/Set/Delete/Has 基础语义（与 Redis API 对齐）
//   - 不做分布式锁 / pub-sub / 脚本（用原生 Redis SDK）
//   - TTL 通过 Option 设置（默认无 TTL = 永久）
//
// 适用场景：
//   - 单进程内存缓存（内置 memory 实现）
//   - 分布式缓存（plugins/cache/redis）
//   - 单测 mock
//
// 不适用：
//   - 强一致性要求（用 RDBMS）
//   - 复杂聚合查询（用 Elasticsearch）
package cache

import (
	"context"
	"time"
)

// Cache 缓存抽象。
//
// 行为契约：
//   - 所有方法首参为 ctx（保留接入点，便于 trace/超时；当前内置实现忽略 ctx 但保留可扩展）
//   - Get 返回 (value, true) 表示命中；(nil, false) 表示未命中或已过期
//   - Set 默认无 TTL（永久）；通过 WithTTL 设置过期
//   - Delete 不存在的 key 是 no-op（不报错）
//   - Has 不更新访问时间（仅探测存在性，可能返回 true 但随后的 Get 返回 false，因后台清理）
//   - Close 释放后台 goroutine（必须调用，避免泄漏）
type Cache interface {
	// Get 按 key 取值；命中返回 (value, true)，未命中或过期返回 (nil, false)
	Get(ctx context.Context, key string) (any, bool)
	// Set 写入 K-V；opts 可指定 TTL
	Set(ctx context.Context, key string, val any, opts ...Option) error
	// Delete 删除 key（不存在 no-op）
	Delete(ctx context.Context, key string) error
	// Has 探测 key 是否存在（不触发懒清理）
	Has(ctx context.Context, key string) bool
	// Close 释放资源（停止后台清理 goroutine）
	Close() error
}

// Item Set 操作的载体
type Item struct {
	Key   string
	Value any
	TTL   time.Duration // 0 = 永久
}

// Option Set 操作的配置函数
type Option func(*Item)

// WithTTL 设置过期时间；0 或负数视为永久
func WithTTL(d time.Duration) Option {
	return func(i *Item) {
		if d > 0 {
			i.TTL = d
		}
	}
}

// NewItem 构造 Item（便于批量调用）
func NewItem(key string, value any, opts ...Option) *Item {
	i := &Item{Key: key, Value: value}
	for _, opt := range opts {
		if opt != nil {
			opt(i)
		}
	}
	return i
}
