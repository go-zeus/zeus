package ratelimit

import "time"

// WaitDuration 等待时长
type WaitDuration struct {
	Allow    bool
	Duration time.Duration
}

// Limiter 限流器接口
type Limiter interface {
	// Allow 判断是否允许请求通过
	Allow() bool
	// Reserve 预留令牌（返回需要等待的时间）
	Reserve() WaitDuration
	// Rate 返回当前速率（每秒允许的请求数）
	Rate() float64
}
