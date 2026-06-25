package counter

import (
	"errors"
	"sync"
	"time"

	"github.com/go-zeus/zeus/circuitbreaker"
)

// ErrCircuitOpen 熔断打开时返回的错误
var ErrCircuitOpen = errors.New("circuitbreaker: circuit is open")

// 编译期检查 countDriver 实现了 circuitbreaker.Breaker 接口
var _ circuitbreaker.Breaker = (*countDriver)(nil)

// Option 计数熔断器选项
type Option func(*countDriver)

// WithThreshold 设置连续失败阈值（达到则打开熔断）
func WithThreshold(threshold int) Option {
	return func(c *countDriver) { c.threshold = threshold }
}

// WithTimeout 设置熔断打开持续时间（过后进入半开）
func WithTimeout(timeout time.Duration) Option {
	return func(c *countDriver) { c.timeout = timeout }
}

// WithHalfOpenMax 设置半开状态最大探测请求数
func WithHalfOpenMax(halfOpenMax int) Option {
	return func(c *countDriver) { c.halfOpenMax = halfOpenMax }
}

type countDriver struct {
	mu          sync.Mutex
	state       circuitbreaker.State
	failures    int
	successes   int
	threshold   int
	timeout     time.Duration
	halfOpenMax int
	halfOpenCnt int
	openedAt    time.Time
}

// New 创建计数熔断器（推荐入口）
//
// threshold: 连续失败次数阈值，达到后打开熔断
// failureRate: 备用参数（占位，目前仅按 threshold 计数；保留以便后续支持率控）
//
// 与 cluster.ClusterBreaker 组合的标准用法：
//
//	cb := clusterbreak.New(func() circuitbreaker.Breaker {
//	    return counter.New(100, 0.5)
//	})
func New(threshold int, failureRate float64) circuitbreaker.Breaker {
	return NewCount(WithThreshold(threshold))
}

// NewCount 创建计数熔断器（带选项的完整构造器）
func NewCount(opts ...Option) circuitbreaker.Breaker {
	c := &countDriver{
		state:       circuitbreaker.StateClosed,
		threshold:   5,
		timeout:     30 * time.Second,
		halfOpenMax: 3,
	}
	for _, opt := range opts {
		opt(c)
	}
	// 校验关键参数：threshold <=0 会导致永不打开熔断，违反熔断器语义
	if c.threshold <= 0 {
		c.threshold = 1
	}
	if c.halfOpenMax <= 0 {
		c.halfOpenMax = 1
	}
	if c.timeout <= 0 {
		c.timeout = 30 * time.Second
	}
	return c
}

func (c *countDriver) Allow() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case circuitbreaker.StateClosed:
		return nil
	case circuitbreaker.StateOpen:
		if time.Since(c.openedAt) > c.timeout {
			c.state = circuitbreaker.StateHalfOpen
			c.halfOpenCnt = 0
			return nil
		}
		return ErrCircuitOpen
	case circuitbreaker.StateHalfOpen:
		if c.halfOpenCnt < c.halfOpenMax {
			c.halfOpenCnt++
			return nil
		}
		return ErrCircuitOpen
	}
	return nil
}

func (c *countDriver) MarkSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case circuitbreaker.StateHalfOpen:
		c.successes++
		if c.successes >= c.halfOpenMax {
			c.state = circuitbreaker.StateClosed
			c.failures = 0
			c.successes = 0
		}
	case circuitbreaker.StateClosed:
		c.failures = 0
	}
}

func (c *countDriver) MarkFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case circuitbreaker.StateHalfOpen:
		c.state = circuitbreaker.StateOpen
		c.openedAt = time.Now()
		c.successes = 0
	case circuitbreaker.StateClosed:
		c.failures++
		if c.failures >= c.threshold {
			c.state = circuitbreaker.StateOpen
			c.openedAt = time.Now()
		}
	}
}

func (c *countDriver) State() circuitbreaker.State {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == circuitbreaker.StateOpen && time.Since(c.openedAt) > c.timeout {
		return circuitbreaker.StateHalfOpen
	}
	return c.state
}
