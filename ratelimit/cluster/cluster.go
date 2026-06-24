// Package cluster 提供按 key（通常是 routing cluster）独立计数的限流器包装器。
//
// 场景：多 cluster 并行开发时，canary 流量不应被 default 流量耗尽的令牌桶限制。
// 每个 cluster 拥有独立的限流桶，互不影响。
//
// 用法：
//
//	import clusterlimit "github.com/go-zeus/zeus/ratelimit/cluster"
//	import "github.com/go-zeus/zeus/ratelimit/token"
//
//	// factory 为每个新 cluster 创建独立 token 桶
//	limiter := clusterlimit.New(func() ratelimit.Limiter {
//	    return token.New(100, 10) // 100 QPS, burst 10
//	})
//
//	// 默认从 ctx 提取 cluster 作为 key
//	limiter.Allow(ctx)
//
//	// 显式 key（不限于 cluster）
//	limiter.AllowKey("tenant-a")
package cluster

import (
	"context"
	"sync"

	"github.com/go-zeus/zeus/ratelimit"
	"github.com/go-zeus/zeus/routing"
)

// LimiterFactory 为每个新 key 创建独立的限流器实例。
// 必须返回非 nil 的 Limiter。
type LimiterFactory func() ratelimit.Limiter

// 编译期检查 ClusterLimiter 实现 ratelimit.Limiter（按 ctx 路由版）
// 注意：ClusterLimiter 不直接实现 ratelimit.Limiter，因为后者无 ctx 参数
// 而是提供 Allow / AllowKey / Reserve / ReserveKey 等扩展 API

// ClusterLimiter 按 key 隔离的多桶限流器
type ClusterLimiter struct {
	factory LimiterFactory
	mu      sync.RWMutex
	buckets map[string]ratelimit.Limiter
}

// New 创建按 key 隔离的限流器。
// factory 决定每个新 key 的限流策略（如 token bucket）。
func New(factory LimiterFactory) *ClusterLimiter {
	if factory == nil {
		factory = func() ratelimit.Limiter { return noopLimiter{} }
	}
	return &ClusterLimiter{
		factory: factory,
		buckets: make(map[string]ratelimit.Limiter),
	}
}

// Allow 判断是否允许通过。key 默认从 ctx 提取 cluster。
func (c *ClusterLimiter) Allow(ctx context.Context) bool {
	return c.AllowKey(routing.FromContext(ctx))
}

// AllowKey 显式指定 key 判断是否允许通过。
func (c *ClusterLimiter) AllowKey(key string) bool {
	return c.bucket(key).Allow()
}

// Reserve 预留令牌。key 默认从 ctx 提取 cluster。
func (c *ClusterLimiter) Reserve(ctx context.Context) ratelimit.WaitDuration {
	return c.ReserveKey(routing.FromContext(ctx))
}

// ReserveKey 显式指定 key 预留令牌。
func (c *ClusterLimiter) ReserveKey(key string) ratelimit.WaitDuration {
	return c.bucket(key).Reserve()
}

// Rate 返回指定 key 的当前速率。
func (c *ClusterLimiter) Rate(key string) float64 {
	return c.bucket(key).Rate()
}

// Keys 返回当前所有已分配桶的 key 列表（用于运维查看）。
func (c *ClusterLimiter) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.buckets))
	for k := range c.buckets {
		keys = append(keys, k)
	}
	return keys
}

// bucket 获取或创建指定 key 的限流器（lazy 初始化）
// 热路径：先 RLock 快速命中，未命中再升级 Lock 创建（双检锁）
func (c *ClusterLimiter) bucket(key string) ratelimit.Limiter {
	c.mu.RLock()
	if l, ok := c.buckets[key]; ok {
		c.mu.RUnlock()
		return l
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// 双检：可能在升级锁期间被其他 goroutine 创建
	if l, ok := c.buckets[key]; ok {
		return l
	}
	l := c.factory()
	if l == nil {
		l = noopLimiter{}
	}
	c.buckets[key] = l
	return l
}

// noopLimiter 兜底实现，factory 返回 nil 时使用
type noopLimiter struct{}

func (noopLimiter) Allow() bool                     { return true }
func (noopLimiter) Reserve() ratelimit.WaitDuration { return ratelimit.WaitDuration{Allow: true} }
func (noopLimiter) Rate() float64                   { return 0 }

// 编译期检查
var _ ratelimit.Limiter = noopLimiter{}
