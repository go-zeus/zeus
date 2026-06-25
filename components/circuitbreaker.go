package components

import (
	"github.com/go-zeus/zeus/circuitbreaker"
)

// CircuitBreakerComponent 熔断器组件适配器
type CircuitBreakerComponent struct {
	cb *circuitbreaker.CircuitBreaker
}

// NewCircuitBreakerComponent 创建熔断器组件
func NewCircuitBreakerComponent(b circuitbreaker.Breaker) *CircuitBreakerComponent {
	return &CircuitBreakerComponent{cb: circuitbreaker.NewCircuitBreaker(b)}
}

func (c *CircuitBreakerComponent) Name() string      { return "circuitbreaker" }
func (c *CircuitBreakerComponent) Depends() []string { return nil }

func (c *CircuitBreakerComponent) Provide(ctx Context) (any, error) {
	return c.cb, nil
}

func (c *CircuitBreakerComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop:  func(ctx Context) error { return nil },
	}
}
