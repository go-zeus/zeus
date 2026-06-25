// Package cluster 提供按 key（通常是 routing cluster）独立熔断的熔断器包装器。
//
// 场景：canary cluster 故障不应触发 default cluster 熔断；每个 cluster 拥有独立熔断器。
//
// 用法：
//
//	import clusterbreak "github.com/go-zeus/zeus/circuitbreaker/cluster"
//	import "github.com/go-zeus/zeus/circuitbreaker/counter"
//
//	cb := clusterbreak.New(func() circuitbreaker.Breaker {
//	    return counter.New(100, 0.5) // 100 请求窗口，50% 失败率触发熔断
//	})
//
//	// 默认从 ctx 提取 cluster
//	err := cb.Execute(ctx, func() error { ... })
package cluster

import (
	"context"
	"sync"

	"github.com/go-zeus/zeus/circuitbreaker"
	"github.com/go-zeus/zeus/routing"
)

// BreakerFactory 为每个新 key 创建独立的 Breaker。
type BreakerFactory func() circuitbreaker.Breaker

// ClusterBreaker 按 key 隔离的多熔断器
type ClusterBreaker struct {
	factory  BreakerFactory
	mu       sync.RWMutex
	breakers map[string]*circuitbreaker.CircuitBreaker
}

// New 创建按 key 隔离的熔断器。
// factory 决定每个新 key 的熔断策略。nil 时使用 alwaysClosed（永不熔断）。
func New(factory BreakerFactory) *ClusterBreaker {
	if factory == nil {
		factory = func() circuitbreaker.Breaker { return alwaysClosed{} }
	}
	return &ClusterBreaker{
		factory:  factory,
		breakers: make(map[string]*circuitbreaker.CircuitBreaker),
	}
}

// Execute 执行函数，自动标记成功/失败。key 默认从 ctx 提取 cluster。
func (c *ClusterBreaker) Execute(ctx context.Context, fn func() error) error {
	return c.ExecuteKey(routing.FromContext(ctx), fn)
}

// ExecuteKey 显式指定 key 执行函数。
func (c *ClusterBreaker) ExecuteKey(key string, fn func() error) error {
	return c.breaker(key).Execute(fn)
}

// AllowKey 判断 key 是否允许通过（不执行 fn）
func (c *ClusterBreaker) AllowKey(key string) error {
	return c.breaker(key).Allow()
}

// StateKey 返回指定 key 的熔断状态
func (c *ClusterBreaker) StateKey(key string) circuitbreaker.State {
	return c.breaker(key).State()
}

// Keys 返回当前所有已分配的 key 列表
func (c *ClusterBreaker) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.breakers))
	for k := range c.breakers {
		keys = append(keys, k)
	}
	return keys
}

// breaker 获取或创建指定 key 的熔断器
// 热路径：先 RLock 快速命中，未命中再升级 Lock 创建（双检锁）
func (c *ClusterBreaker) breaker(key string) *circuitbreaker.CircuitBreaker {
	c.mu.RLock()
	if cb, ok := c.breakers[key]; ok {
		c.mu.RUnlock()
		return cb
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// 双检：可能在升级锁期间被其他 goroutine 创建
	if cb, ok := c.breakers[key]; ok {
		return cb
	}
	b := c.factory()
	if b == nil {
		b = alwaysClosed{}
	}
	cb := circuitbreaker.NewCircuitBreaker(b)
	c.breakers[key] = cb
	return cb
}

// alwaysClosed factory 返回 nil 时的兜底
type alwaysClosed struct{}

func (alwaysClosed) Allow() error                { return nil }
func (alwaysClosed) MarkSuccess()                {}
func (alwaysClosed) MarkFailed()                 {}
func (alwaysClosed) State() circuitbreaker.State { return circuitbreaker.StateClosed }

var _ circuitbreaker.Breaker = alwaysClosed{}
