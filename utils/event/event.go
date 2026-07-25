package event

import "sync"

// dropPolicy 决定 watcher 尚有未消费事件时，新触发如何处理。
type dropPolicy int

const (
	// keepLatest（默认）：清空 watcher 已有的未消费事件后写入最新触发。
	// burst 场景下 watcher 总能看到最近一次触发（事件合并）。
	keepLatest dropPolicy = iota
	// keepOldest：watcher 已有未消费事件时丢弃后续触发，保留首个未消费事件。
	// 适合"不能错过第一个信号"的场景。
	keepOldest
)

// Option 配置 Event。
type Option func(*config)

type config struct {
	policy dropPolicy
}

// WithKeepOldest 设置"保留最早"策略：watcher 尚有未消费事件时丢弃后续触发。
// 默认为"保留最新"（合并 burst，watcher 看到最近一次触发）。
func WithKeepOldest() Option {
	return func(c *config) { c.policy = keepOldest }
}

// Event 多次触发事件，支持多 watcher 独立接收。
//
// 每个 Watch 返回独立的 buffered(1) channel；Trigger 时 fan-out 到所有 watcher，
// 避免共享 channel 导致 watcher 之间"抢"事件。
//
// 触发频率高于消费频率时的丢弃策略由 Option 控制：默认保留最新（NewEvent），
// 或 NewEvent(WithKeepOldest()) 保留最早。
type Event interface {
	Trigger()               // 触发事件，多次调用安全
	Watch() <-chan struct{} // 返回独立通道，每次触发后可接收一次（受丢弃策略影响）
	Close()                 // 关闭事件，释放所有 watcher channel
}

// NewEvent 创建多次触发事件。
//
// opts：
//   - 默认（无 Option）：保留最新触发（合并 burst）
//   - WithKeepOldest()：保留首个未消费事件，丢弃后续
func NewEvent(opts ...Option) Event {
	cfg := config{policy: keepLatest}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &eventImpl{
		watchers: make(map[chan struct{}]struct{}),
		policy:   cfg.policy,
	}
}

type eventImpl struct {
	mu       sync.Mutex
	watchers map[chan struct{}]struct{}
	policy   dropPolicy
	closed   bool
}

func (e *eventImpl) Trigger() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	for ch := range e.watchers {
		switch e.policy {
		case keepLatest:
			// 清空旧事件，确保最新一次触发能被接收
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- struct{}{}:
			default:
			}
		case keepOldest:
			// 已有未消费事件则丢弃本次，保留首个
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

func (e *eventImpl) Watch() <-chan struct{} {
	ch := make(chan struct{}, 1)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		close(ch)
		return ch
	}
	e.watchers[ch] = struct{}{}
	return ch
}

func (e *eventImpl) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	for ch := range e.watchers {
		close(ch)
		delete(e.watchers, ch)
	}
}

// —— 向后兼容别名（已收敛到 Event + Option，建议迁移）——

// OneEvent 等价于 Event（保留最早策略）。
//
// Deprecated: 使用 NewEvent(WithKeepOldest()) 和 Event 类型代替。
type OneEvent = Event

// NewOneEvent 创建保留最早策略的事件。
//
// Deprecated: 使用 NewEvent(WithKeepOldest()) 代替。
func NewOneEvent() Event {
	return NewEvent(WithKeepOldest())
}
