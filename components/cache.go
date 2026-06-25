package components

import (
	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/log"
)

// CacheComponent 缓存组件适配器。
//
// 职责：
//   - 持有 cache.Cache 实例
//   - OnStop 时 Close 释放后台 goroutine
//
// 与底层实现解耦：用户可在 New 时注入任意 cache.Cache 实现
// （cache/memory 或 plugins/cache/redis）。
//
// 用法：
//
//	c := memory.New(memory.WithTracer(tracer), memory.WithMeter(meter))
//	app := components.NewApp(
//	    components.NewCacheComponent(c),
//	    // 业务组件通过 Type[cache.Cache](ctx) 取 cache 实例
//	)
type CacheComponent struct {
	cache cache.Cache
}

// NewCacheComponent 创建缓存组件
//
// cache 为 nil 时返回的组件为 no-op
func NewCacheComponent(c cache.Cache) *CacheComponent {
	return &CacheComponent{cache: c}
}

func (c *CacheComponent) Name() string      { return "cache" }
func (c *CacheComponent) Depends() []string { return nil }

// Provide 把 Cache 实例发布到容器
func (c *CacheComponent) Provide(_ Context) (any, error) {
	return c.cache, nil
}

// Lifecycle OnStart 仅打日志；OnStop 调 Close 释放后台 goroutine
func (c *CacheComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(_ Context) error {
			if c.cache != nil {
				log.Info("cache ready")
			}
			return nil
		},
		OnStop: func(_ Context) error {
			if c.cache == nil {
				return nil
			}
			return c.cache.Close()
		},
	}
}
