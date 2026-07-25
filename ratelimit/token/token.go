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
// burst: 桶容量（>0）
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
	}
	// 选项应用后再统一赋初值（满桶），与 New 一致。
	// 原实现字面量 tokens=rate 导致初始半桶/空桶，违反"初始满桶"承诺，首请求可能被错误拒绝。
	t.tokens = float64(t.burst)
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
	// 预占一个未来令牌：tokens 减到负值，refill 从负值爬升，后续 Allow/Reserve 必须等
	// 这枚预占被时间补回（标准 guava / golang.org/x/time/rate 语义，防超卖）。
	// 原实现不扣减 tokens，N 个并发 Reserve 各自等待后全部放行，限流彻底失效。
	waitTokens := 1 - t.tokens
	t.tokens -= 1
	return ratelimit.WaitDuration{Allow: true, Duration: time.Duration((waitTokens / t.rate) * float64(time.Second))}
}

func (t *tokenLimiter) Rate() float64 {
	return t.rate
}
