package exponential

import (
	"math"
	"sync"
	"time"

	"github.com/go-zeus/zeus/retry"
)

// Option 指数退避选项
type Option func(*exponentialRetrier)

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(max int) Option {
	return func(e *exponentialRetrier) { e.maxRetries = max }
}

// WithBaseDelay 设置基础延迟
func WithBaseDelay(base time.Duration) Option {
	return func(e *exponentialRetrier) { e.baseDelay = base }
}

// WithMaxDelay 设置最大延迟
func WithMaxDelay(max time.Duration) Option {
	return func(e *exponentialRetrier) { e.maxDelay = max }
}

type exponentialRetrier struct {
	mu         sync.Mutex
	count      int
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// New 创建指数退避重试策略
//
// 标准用法（推荐位置参数）：
//
//	r := exponential.New(3, 100*time.Millisecond) // 3 次重试，初始 100ms
//
// 也支持选项模式：
//
//	r := exponential.NewWithOptions(exponential.WithMaxRetries(5))
func New(maxRetries int, baseDelay time.Duration) retry.Retrier {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	return &exponentialRetrier{
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		maxDelay:   10 * time.Second,
	}
}

// NewWithOptions 带完整选项的构造器
func NewWithOptions(opts ...Option) retry.Retrier {
	e := &exponentialRetrier{
		maxRetries: 3,
		baseDelay:  100 * time.Millisecond,
		maxDelay:   10 * time.Second,
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.maxRetries < 0 {
		e.maxRetries = 0
	}
	if e.baseDelay <= 0 {
		e.baseDelay = 100 * time.Millisecond
	}
	if e.maxDelay <= 0 {
		e.maxDelay = 10 * time.Second
	}
	return e
}

func (e *exponentialRetrier) Next() (time.Duration, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.count >= e.maxRetries {
		return 0, false
	}
	// float 域比较 + IsInf/IsNaN 防护：大 count 时 math.Pow(2, count) 返回 +Inf，
	// time.Duration(+Inf) 经饱和转换为 MinInt64（负值），原 `delay > maxDelay` 对负数不成立，
	// 导致退避归零、重试风暴。先在 float 域 clamp 再转 Duration。
	d := float64(e.baseDelay) * math.Pow(2, float64(e.count))
	if math.IsInf(d, 1) || math.IsNaN(d) || d > float64(e.maxDelay) {
		d = float64(e.maxDelay)
	}
	delay := time.Duration(d)
	e.count++
	return delay, true
}

func (e *exponentialRetrier) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.count = 0
}

func (e *exponentialRetrier) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.count
}
