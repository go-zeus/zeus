// Package batch 提供通用批处理（Batcher）工具。
//
// 设计目标：
//   - 泛型支持任意 payload 类型
//   - 双触发：最大批量 + 最大等待时间（先到先触发）
//   - 线程安全：基于 channel + sync.Mutex
//   - 优雅关闭：Flush 残留批次
//   - 零依赖（仅用标准库）
//
// 用例：
//   - 数据库批量插入（减少 RTT）
//   - 日志批量写入（减少 I/O）
//   - 消息批量发送（提升吞吐）
//
// 使用示例：
//
//	b := batch.New(func(items []string) {
//	    db.Insert(items)
//	}, batch.WithMaxBatchSize(100), batch.WithMaxWait(100*time.Millisecond))
//	defer b.Close()
//
//	for _, s := range data {
//	    b.Add(s) // 高频 Add，低频 Insert
//	}
package batch

import (
	"context"
	"sync"
	"time"
)

// —— 默认值 ——

const (
	defaultMaxBatchSize = 100
	defaultMaxWait      = 100 * time.Millisecond
)

// —— 非泛型 Option ——

// config 内部配置（用户不直接操作）
type config struct {
	maxSize int
	maxWait time.Duration
}

// Option 批处理配置（非泛型，避免类型推断困难）
type Option func(*config)

// WithMaxBatchSize 设置单批次最大元素数（达到后立即触发）
//
// 默认 100
func WithMaxBatchSize(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxSize = n
		}
	}
}

// WithMaxWait 设置最大等待时间（超过后触发，即使批次未满）
//
// 默认 100ms
// 设为 0 表示禁用基于时间的触发（仅靠 size 或 Flush）
func WithMaxWait(d time.Duration) Option {
	return func(c *config) {
		c.maxWait = d
	}
}

// Batcher 批处理器
//
// 模型：
//   - Add(item) 把元素塞入当前 batch
//   - 当 batch 长度达到 MaxBatchSize 或距离上次 flush 超过 MaxWait 时，触发 handler
//   - handler 在独立 goroutine 中执行，避免阻塞 Add
//
// 注意：handler 必须是线程安全的（多批次可能并发执行）
type Batcher[T any] struct {
	handler func([]T)

	maxSize int
	maxWait time.Duration

	queue   chan T
	flushCh chan struct{}
	stopCh  chan struct{}
	stopped chan struct{}

	wg sync.WaitGroup

	// mu/pending 供 Flush 同步使用（外部主动触发时等待后台 worker 完成）
	mu      sync.Mutex
	pending []T
}

// New 创建批处理器
//
// handler 在每个批次完成时调用；如果 handler panic，batcher 会捕获并继续（不会崩溃）
// 必须调用 b.Close() 释放资源（defer 模式）
func New[T any](handler func([]T), opts ...Option) *Batcher[T] {
	cfg := &config{
		maxSize: defaultMaxBatchSize,
		maxWait: defaultMaxWait,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	b := &Batcher[T]{
		handler: handler,
		maxSize: cfg.maxSize,
		maxWait: cfg.maxWait,
		queue:   make(chan T, 1024),
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
		pending: make([]T, 0, cfg.maxSize),
	}

	b.wg.Add(1)
	go b.run()
	return b
}

// Add 添加元素到当前批次（非阻塞，背压控制）
//
// 行为：
//   - 一般情况下立即入队
//   - 内部队列已满（1024 项）时阻塞（自然背压）
func (b *Batcher[T]) Add(item T) {
	select {
	case <-b.stopCh:
		return // 已停止，丢弃
	default:
	}
	b.queue <- item
}

// TryAdd 非阻塞添加，返回是否成功
//
// 用途：高频场景下不希望被背压阻塞，丢弃部分元素
func (b *Batcher[T]) TryAdd(item T) bool {
	select {
	case b.queue <- item:
		return true
	default:
		return false
	}
}

// AddContext 带上下文的 Add
//
// ctx 取消时返回 ctx.Err()
// 优先检查 ctx 是否已取消，避免与可发送的 queue 在 select 中随机竞争
func (b *Batcher[T]) AddContext(ctx context.Context, item T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-b.stopCh:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case b.queue <- item:
		return nil
	}
}

// Flush 立即同步触发当前批次（即使未满）
//
// 行为：
//   - 先从 queue 中 drain 所有未处理的元素到 pending
//   - 取出 pending 全部元素
//   - 在调用方 goroutine 中同步执行 handler
//   - 与后台 run loop 互斥（通过 mu 保护，不会重复处理同一 batch）
//
// 适用：测试断言 / 优雅关闭场景
func (b *Batcher[T]) Flush() {
	b.mu.Lock()
	// drain queue 中所有就绪元素到 pending（与 run loop 竞争，互不影响）
	draining := true
	for draining {
		select {
		case item := <-b.queue:
			b.pending = append(b.pending, item)
		default:
			draining = false
		}
	}
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.pending
	b.pending = make([]T, 0, b.maxSize)
	b.mu.Unlock()

	b.callHandler(batch)
}

// Close 关闭批处理器
//
// 行为：
//  1. 关闭 stopCh，停止接收新元素
//  2. 等待当前批次 flush 完成
//  3. 关闭 stopped 通道
//
// 调用后 Add 会被丢弃
func (b *Batcher[T]) Close() {
	select {
	case <-b.stopCh:
		return // 已关闭
	default:
		close(b.stopCh)
	}
	b.wg.Wait()
}

// —— 内部 ——

func (b *Batcher[T]) run() {
	defer b.wg.Done()
	defer close(b.stopped)

	var timer *time.Timer
	var timerC <-chan time.Time

	if b.maxWait > 0 {
		timer = time.NewTimer(b.maxWait)
		timerC = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-b.stopCh:
			// 关闭前先 flush 所有 pending
			b.drainAndFlush()
			return

		case item := <-b.queue:
			b.mu.Lock()
			b.pending = append(b.pending, item)
			shouldFlush := len(b.pending) >= b.maxSize
			b.mu.Unlock()

			if shouldFlush {
				b.flushNow()
			}

		case <-b.flushCh:
			b.flushNow()

		case <-timerC:
			b.flushNow()
			// 重置 timer
			if b.maxWait > 0 {
				timer.Reset(b.maxWait)
			}
		}
	}
}

// flushNow 取出 pending 并调用 handler
func (b *Batcher[T]) flushNow() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.pending
	b.pending = make([]T, 0, b.maxSize)
	b.mu.Unlock()

	// 在锁外调用 handler 避免阻塞其他 Add
	b.callHandler(batch)
}

// drainAndFlush 关闭时 flush 所有 queue 中残留元素
func (b *Batcher[T]) drainAndFlush() {
	// 先 drain queue 到 pending
	for {
		select {
		case item := <-b.queue:
			b.mu.Lock()
			b.pending = append(b.pending, item)
			if len(b.pending) >= b.maxSize {
				batch := b.pending
				b.pending = make([]T, 0, b.maxSize)
				b.mu.Unlock()
				b.callHandler(batch)
				continue
			}
			b.mu.Unlock()
		default:
			// queue 空，flush 最后的 pending
			b.flushNow()
			return
		}
	}
}

// callHandler 安全调用 handler（捕获 panic）
func (b *Batcher[T]) callHandler(batch []T) {
	if len(batch) == 0 {
		return
	}
	defer func() {
		_ = recover() // 捕获 panic 避免崩溃
	}()
	b.handler(batch)
}
