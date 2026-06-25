package components

import (
	"github.com/go-zeus/zeus/retry"
)

// RetryComponent 重试策略组件适配器
type RetryComponent struct {
	r retry.Retrier
}

// NewRetryComponent 创建重试策略组件
func NewRetryComponent(r retry.Retrier) *RetryComponent {
	return &RetryComponent{r: r}
}

func (r *RetryComponent) Name() string      { return "retry" }
func (r *RetryComponent) Depends() []string { return nil }

func (r *RetryComponent) Provide(ctx Context) (any, error) {
	return r.r, nil
}

func (r *RetryComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop:  func(ctx Context) error { return nil },
	}
}
