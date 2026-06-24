// Package cluster 提供按 key（通常是 routing cluster）独立的重试器包装器。
//
// 场景：集群路由下，对同一 cluster 内的重试应使用该 cluster 的重试策略（如不同 cluster 配置不同超时）。
// 注意：Retrier 通常按请求实例化（每次调用 Next 推进游标），故本包装器
// 提供"按 key 工厂"模式——每次调用返回一个全新的 Retrier 实例。
//
// 用法：
//
//	import clusterretry "github.com/go-zeus/zeus/retry/cluster"
//	import "github.com/go-zeus/zeus/retry/exponential"
//
//	r := clusterretry.New(func() retry.Retrier {
//	    return exponential.New(3, 100*time.Millisecond)
//	})
//
//	// 默认从 ctx 提取 cluster
//	retrier := r.NewRetriever(ctx)
//	for retrier.Next() { ... }
package cluster

import (
	"context"
	"sync"
	"time"

	"github.com/go-zeus/zeus/retry"
	"github.com/go-zeus/zeus/routing"
)

// RetrierFactory 为每次重试序列创建独立 Retrier。
type RetrierFactory func() retry.Retrier

// ClusterRetrier 按 key 选择不同的 RetrierFactory
type ClusterRetrier struct {
	mu        sync.RWMutex
	factories map[string]RetrierFactory
	default_  RetrierFactory
}

// New 创建按 key 路由的重试器。
// defaultFactory 为没有匹配 key 时使用，nil 时使用 noRetry（立即停止）。
func New(defaultFactory RetrierFactory) *ClusterRetrier {
	if defaultFactory == nil {
		defaultFactory = func() retry.Retrier { return noRetry{} }
	}
	return &ClusterRetrier{
		factories: make(map[string]RetrierFactory),
		default_:  defaultFactory,
	}
}

// Set 为指定 key 设置专用 RetrierFactory
func (c *ClusterRetrier) Set(key string, factory RetrierFactory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.factories[key] = factory
}

// NewRetriever 返回一个新的 Retrier。key 默认从 ctx 提取 cluster。
func (c *ClusterRetrier) NewRetriever(ctx context.Context) retry.Retrier {
	return c.NewRetrieverForKey(routing.FromContext(ctx))
}

// NewRetrieverForKey 显式指定 key 返回 Retrier。
// 无匹配 key 时使用 default factory。
func (c *ClusterRetrier) NewRetrieverForKey(key string) retry.Retrier {
	c.mu.RLock()
	f, ok := c.factories[key]
	c.mu.RUnlock()
	if !ok {
		f = c.default_
	}
	if f == nil {
		return noRetry{}
	}
	r := f()
	if r == nil {
		return noRetry{}
	}
	return r
}

// noRetry 立即停止的兜底 Retrier
type noRetry struct{}

func (noRetry) Next() (time.Duration, bool) { return 0, false }
func (noRetry) Reset()                      {}
func (noRetry) Count() int                  { return 0 }

var _ retry.Retrier = noRetry{}
