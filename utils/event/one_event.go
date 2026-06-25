package event

import "sync"

// OneEvent 在消费方未完成的时间里只保留一个事件
//
// 设计要点：每个 Watch 返回独立 channel，Trigger 时 fan-out 到所有 watcher
type OneEvent interface {
	Trigger()               // 发送事件，如果 chan 里有一个事件则触发的事件会丢弃
	Watch() <-chan struct{} // 监听（返回独立 channel）
	Close()                 // 关闭
}

func NewOneEvent() OneEvent {
	return &oneEvent{
		watchers: make(map[chan struct{}]struct{}),
	}
}

type oneEvent struct {
	mu       sync.Mutex
	watchers map[chan struct{}]struct{}
	closed   bool
}

func (d *oneEvent) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	for ch := range d.watchers {
		select {
		case ch <- struct{}{}:
		default:
			// 该 watcher 还有未消费事件，丢弃本次
		}
	}
}

func (d *oneEvent) Watch() <-chan struct{} {
	ch := make(chan struct{}, 1)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		close(ch)
		return ch
	}
	d.watchers[ch] = struct{}{}
	return ch
}

func (d *oneEvent) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	for ch := range d.watchers {
		close(ch)
		delete(d.watchers, ch)
	}
}
