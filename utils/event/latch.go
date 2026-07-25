package event

import (
	"sync"
	"sync/atomic"
)

// Latch 一次性事件：首次 Trigger 后永久 Done，供多个等待方观察同一个完成信号。
//
// 类似 context.Done，但由业务手动触发，且可查询 HasFired。
// 命名 Latch（锁存器）以区别于多次触发的 Event，消除原 OnceEvent/OneEvent 的命名混淆。
type Latch interface {
	Trigger() bool         // 触发（仅首次生效），返回是否由本次调用触发
	Done() <-chan struct{} // 返回通道，Trigger 后关闭
	HasFired() bool        // 是否已触发
}

type latch struct {
	triggered int32
	c         chan struct{}
	o         sync.Once
}

// NewLatch 创建一次性事件。
func NewLatch() Latch {
	return &latch{c: make(chan struct{})}
}

func (l *latch) Trigger() bool {
	ret := false
	l.o.Do(func() {
		atomic.StoreInt32(&l.triggered, 1)
		close(l.c)
		ret = true
	})
	return ret
}

func (l *latch) Done() <-chan struct{} { return l.c }

func (l *latch) HasFired() bool { return atomic.LoadInt32(&l.triggered) == 1 }

// —— 向后兼容别名（建议迁移到 Latch）——

// OnceEvent 等价于 Latch。
//
// Deprecated: 使用 NewLatch() 和 Latch 类型代替，避免与 OneEvent 混淆。
type OnceEvent = Latch

// NewOnceEvent 创建一次性事件。
//
// Deprecated: 使用 NewLatch() 代替。
func NewOnceEvent() Latch {
	return NewLatch()
}
