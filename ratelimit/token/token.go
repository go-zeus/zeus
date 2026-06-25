package token

import (
	"sync"
	"time"

	"github.com/go-zeus/zeus/ratelimit"
)

// Option 令牌桶选项
type Option func(*tokenLimiter)

// WithRate 设置每秒产生令牌数
func WithRate(rate float64) Option {
	return func(t *tokenLimiter) { t.rate = rate }
}

// WithBurst 设置桶容量
func WithBurst(burst int) Option {
	return func(t *tokenLimiter) { t.burst = burst }
}

type tokenLimiter struct {
	mu       sync.Mutex
	rate     float64 // 每秒产生令牌数
	burst    int     // 桶容量
	tokens   float64
	lastTime time.Time
}

// New 创建令牌桶限流器
//
// rate: 每秒产生令牌数（>0）
// burst: 桶容量（>0）；如 burst < rate，桶大小会被自动调整为 max(burst, 1)
//
// 标准用法：
//
//	limiter := token.New(100, 10) // 100 QPS, burst 10
func New(rate float64, burst int) ratelimit.Limiter {
	t := &tokenLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   float64(burst), // 初始满桶，避免首次请求被拒
		lastTime: time.Now(),
	}
	// 参数校验：rate <=0 会导致桶永不补充；burst <=0 会让首次请求即拒绝
	if t.rate <= 0 {
		t.rate = 1
	}
	if t.burst <= 0 {
		t.burst = 1
		t.tokens = 1
	}
	return t
}

// NewWithOptions 带完整选项的构造器（保留原 NewCount 风格）
// 若无需自定义选项，直接使用 New
func NewWithOptions(rate float64, opts ...Option) ratelimit.Limiter {
	t := &tokenLimiter{
		rate:     rate,
		burst:    int(rate),
		tokens:   rate,
		lastTime: time.Now(),
	}
	for _, opt := range opts {
		opt(t)
	}
	if t.rate <= 0 {
		t.rate = 1
	}
	if t.burst <= 0 {
		t.burst = 1
		t.tokens = float64(t.burst)
	}
	return t
}

func (t *tokenLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(t.lastTime).Seconds()
	t.tokens += elapsed * t.rate
	if t.tokens > float64(t.burst) {
		t.tokens = float64(t.burst)
	}
	t.lastTime = now
}

func (t *tokenLimiter) Allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refill()
	if t.tokens >= 1 {
		t.tokens--
		return true
	}
	return false
}

func (t *tokenLimiter) Reserve() ratelimit.WaitDuration {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refill()
	if t.tokens >= 1 {
		t.tokens--
		return ratelimit.WaitDuration{Allow: true, Duration: 0}
	}
	delay := (1 - t.tokens) / t.rate
	return ratelimit.WaitDuration{Allow: true, Duration: time.Duration(delay * float64(time.Second))}
}

func (t *tokenLimiter) Rate() float64 {
	return t.rate
}
