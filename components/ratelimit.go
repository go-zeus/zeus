package components

import (
	"github.com/go-zeus/zeus/ratelimit"
)

// RateLimitComponent 限流器组件适配器
type RateLimitComponent struct {
	rl ratelimit.Limiter
}

// NewRateLimitComponent 创建限流器组件
func NewRateLimitComponent(rl ratelimit.Limiter) *RateLimitComponent {
	return &RateLimitComponent{rl: rl}
}

func (r *RateLimitComponent) Name() string      { return "ratelimit" }
func (r *RateLimitComponent) Depends() []string { return nil }

func (r *RateLimitComponent) Provide(ctx Context) (any, error) {
	return r.rl, nil
}

func (r *RateLimitComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop:  func(ctx Context) error { return nil },
	}
}
