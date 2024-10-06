package event

import (
	"sync"
	"sync/atomic"
)

// OnceEvent 表示将来可能发生的一次性事件。
type OnceEvent interface {
	Trigger() bool         //触发事件，多次调用安全
	Done() <-chan struct{} //返回一个通道，Trigger调用后关闭
	HasFired() bool        //是否调用过Trigger
}

type onceEvent struct {
	triggered int32
	c         chan struct{}
	o         sync.Once
}

// NewOnceEvent 创建事件
func NewOnceEvent() OnceEvent {
	return &onceEvent{c: make(chan struct{})}
}

// Trigger 触发事件，多次调用安全
func (e *onceEvent) Trigger() bool {
	ret := false
	e.o.Do(func() {
		atomic.StoreInt32(&e.triggered, 1)
		close(e.c)
		ret = true
	})
	return ret
}

// Done 返回一个通道，Trigger调用后关闭
func (e *onceEvent) Done() <-chan struct{} {
	return e.c
}

// HasFired 是否调用过Trigger
func (e *onceEvent) HasFired() bool {
	return atomic.LoadInt32(&e.triggered) == 1
}
