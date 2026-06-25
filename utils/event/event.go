package event

import "sync"

// Event 通用事件，支持多次触发
//
// 设计要点：每个 Watch 返回独立 channel，Trigger 时 fan-out 到所有 watcher
// 避免共享 channel 导致多 watcher 之间"抢"事件
type Event interface {
	Trigger()               // 触发事件，多次调用安全
	Watch() <-chan struct{} // 返回一个独立通道，每次 Trigger 后可接收一次
	Close()                 // 关闭事件，释放所有 watcher channel
}

func NewEvent() Event {
	return &event{
		watchers: make(map[chan struct{}]struct{}),
	}
}

type event struct {
	mu       sync.Mutex
	watchers map[chan struct{}]struct{}
	closed   bool
}

func (e *event) Trigger() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	for ch := range e.watchers {
		// 清空旧事件，确保最新一次触发能被接收
		select {
		case <-ch:
		default:
		}
		// 非阻塞写入；watcher 未消费时丢弃事件，避免阻塞 Trigger
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (e *event) Watch() <-chan struct{} {
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

func (e *event) Close() {
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
