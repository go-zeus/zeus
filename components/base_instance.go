package components

import "sync"

type BaseInstance struct {
	mu    sync.Mutex // 为条件变量提供锁定
	cond  *sync.Cond
	ready bool
}

func (b *BaseInstance) SetReady() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ready = true
	b.cond.Broadcast() // 唤醒所有等待的goroutine
}

func (b *BaseInstance) IsReady() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ready
}

func (b *BaseInstance) Wait() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for !b.ready {
		b.cond.Wait()
	}
}
