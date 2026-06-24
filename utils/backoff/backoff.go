// Package backoff 提供重试退避算法的通用实现。
//
// 设计目的：
//   - 抽象 Backoff 接口，支持任意算法（指数退避 / 固定间隔 / 自定义）
//   - 内置 Exponential（指数退避 + jitter）与 Constant（固定间隔）实现
//   - 不绑定 retry 框架，可独立用于任何"失败后等待"场景
//
// 与 retry 包的关系：
//   - retry 是上层框架（提供 Retrier / Retriever 抽象，含 cluster 隔离）
//   - backoff 是底层算法（只关心"下次等多久"）
//   - retry 包未来可注入 Backoff 接口替换其硬编码算法
//
// 并发安全：
//   - 单个 Backoff 实例非线程安全（attempt 计数非原子）
//   - 跨 goroutine 使用请每次重试创建新实例（标准做法）
package backoff

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// Backoff 退避算法接口
//
// 行为约定：
//   - Next() 返回下次重试前的等待时间
//   - Reset() 重置 attempt 计数为 0
//   - 多次调用 Next() 应单调递增（或保持上限）
type Backoff interface {
	// Next 返回下次等待时间，并使 attempt++
	Next() time.Duration
	// Reset 重置 attempt 计数
	Reset()
}

// —— Exponential 指数退避 ——

// Exponential 指数退避：base * 2^attempt，上限 max，可选 jitter
//
// 公式：next = min(base * 2^attempt, max) ± jitter
//
// 适用场景：网络重试、限流恢复、连接重建
type Exponential struct {
	base    time.Duration // 初始间隔（attempt=0 时）
	max     time.Duration // 单次等待上限
	factor  float64       // 增长因子（默认 2.0）
	jitter  float64       // jitter 比例 [0, 1]，0 表示无抖动
	attempt int           // 当前重试次数
	mu      sync.Mutex
	rand    *rand.Rand
}

// NewExponential 创建指数退避器
//
// 参数：
//   - base: 初始间隔（attempt=0 时返回 base）
//   - opts: 可选配置（WithMax / WithFactor / WithJitter）
func NewExponential(base time.Duration, opts ...Option) *Exponential {
	e := &Exponential{
		base:   base,
		max:    time.Duration(math.MaxInt64),
		factor: 2.0,
		jitter: 0,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Next 返回下次等待时间
func (e *Exponential) Next() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 计算基础退避：base * factor^attempt
	backoff := float64(e.base)
	for i := 0; i < e.attempt; i++ {
		backoff *= e.factor
	}

	// 上限钳制
	if maxF := float64(e.max); backoff > maxF {
		backoff = maxF
	}

	// jitter：在 [1-jitter, 1+jitter] 范围内随机扰动
	if e.jitter > 0 {
		if e.rand == nil {
			e.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		delta := backoff * e.jitter
		backoff = backoff - delta + e.rand.Float64()*(2*delta)
	}

	e.attempt++
	return time.Duration(backoff)
}

// Reset 重置 attempt 计数
func (e *Exponential) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.attempt = 0
}

// Attempt 返回当前 attempt 数（已调用的 Next 次数）
func (e *Exponential) Attempt() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.attempt
}

// —— Constant 固定间隔 ——

// Constant 固定间隔：每次返回相同的 interval
//
// 适用场景：固定节奏的轮询、健康检查
type Constant struct {
	interval time.Duration
	attempt  int
	mu       sync.Mutex
}

// NewConstant 创建固定间隔退避器
func NewConstant(interval time.Duration) *Constant {
	return &Constant{interval: interval}
}

// Next 返回固定间隔
func (c *Constant) Next() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempt++
	return c.interval
}

// Reset 重置 attempt 计数（interval 不变）
func (c *Constant) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempt = 0
}

// Attempt 返回当前 attempt 数
func (c *Constant) Attempt() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempt
}

// —— Option ——

// Option 配置 Exponential 实例
type Option func(*Exponential)

// WithMax 设置单次等待上限
//
// 默认：math.MaxInt64（实际不限制）
//
// 用途：避免重试间隔无限增长（如 base=1ms, factor=2, attempt=40 时理论达 1TB ms）
func WithMax(max time.Duration) Option {
	return func(e *Exponential) { e.max = max }
}

// WithFactor 设置增长因子
//
// 默认：2.0（每次翻倍）
//
// 推荐：
//   - 2.0：典型指数退避
//   - 1.5：缓慢增长（适合低延迟场景）
//   - 3.0：快速增长（适合明确知道有较长恢复时间）
func WithFactor(factor float64) Option {
	return func(e *Exponential) { e.factor = factor }
}

// WithJitter 设置抖动比例
//
// 参数：jitter ∈ [0, 1]
//   - 0：无抖动（确定性退避，可能导致惊群效应）
//   - 0.1：±10% 抖动（推荐）
//   - 1.0：完全随机（实际退避在 [0, 2*base] 间）
//
// 用途：避免多个客户端同时重试造成"惊群"（thundering herd）
func WithJitter(jitter float64) Option {
	return func(e *Exponential) {
		if jitter < 0 {
			jitter = 0
		}
		if jitter > 1 {
			jitter = 1
		}
		e.jitter = jitter
	}
}

// —— 通用工具 ——

// Sleep 阻塞 sleep 一段 backoff 时间，可被 ctx 取消
//
// 返回 nil 表示等待完成，error 表示 ctx 提前取消
//
// 示例：
//
//	for retry {
//	    if err := op(); err == nil { break }
//	    if err := backoff.Sleep(ctx, b); err != nil { return err }
//	}
func Sleep(ctx interface{ Done() <-chan struct{} }, b Backoff) error {
	d := b.Next()
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctxDoneErr(ctx)
	}
}

// ctxDoneErr 提取 ctx 的 Err() —— 用 interface 类型避免硬依赖 context 包
type ctxErr interface {
	Err() error
}

func ctxDoneErr(ctx interface{ Done() <-chan struct{} }) error {
	if e, ok := ctx.(ctxErr); ok {
		return e.Err()
	}
	return nil
}
