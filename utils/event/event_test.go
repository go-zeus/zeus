package event

import (
	"testing"
	"time"
)

// TestEvent_Subscribe 验证 Watch 返回的通道在 Trigger 后可接收事件
func TestEvent_Subscribe(t *testing.T) {
	e := NewEvent()
	ch := e.Watch()
	e.Trigger()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Error("Watch() 后 Trigger，应在1秒内收到事件")
	}
}

// TestEvent_MultipleSubscribers 多 watcher 各自独立通道均能收到
func TestEvent_MultipleSubscribers(t *testing.T) {
	e := NewEvent()
	ch1, ch2 := e.Watch(), e.Watch()
	if ch1 == ch2 {
		t.Error("多次 Watch 应返回独立通道")
	}
	e.Trigger()
	for i, ch := range []<-chan struct{}{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Errorf("订阅者 %d 未收到事件", i+1)
		}
	}
}

// 默认策略 keepLatest：连触多次合并为最新一次
func TestEvent_KeepLatest(t *testing.T) {
	e := NewEvent()
	ch := e.Watch()
	e.Trigger()
	e.Trigger()
	e.Trigger()
	select {
	case <-ch:
	default:
		t.Error("keepLatest: watcher 应至少收到一次")
	}
	// 3 次触发合并为 1 个待消费事件，再读应阻塞
	select {
	case <-ch:
		t.Error("keepLatest: 3 次触发应合并为 1 个待消费事件")
	default:
	}
}

// keepOldest 策略：连触多次只保留首个
func TestEvent_KeepOldest(t *testing.T) {
	e := NewEvent(WithKeepOldest())
	ch := e.Watch()
	e.Trigger()
	e.Trigger()
	e.Trigger()
	select {
	case <-ch:
	default:
		t.Error("keepOldest: watcher 应收到首个事件")
	}
	select {
	case <-ch:
		t.Error("keepOldest: 后续触发应被丢弃")
	default:
	}
}

func TestEvent_Close_ReleasesWatchers(t *testing.T) {
	e := NewEvent()
	ch1, ch2 := e.Watch(), e.Watch()
	e.Close()
	for i, ch := range []<-chan struct{}{ch1, ch2} {
		select {
		case <-ch:
		default:
			t.Errorf("订阅者 %d 的 channel 应被 Close 关闭", i+1)
		}
	}
	e.Trigger() // Close 后 Trigger 不 panic
	// Close 后 Watch 返回已关闭 channel
	ch3 := e.Watch()
	if _, ok := <-ch3; ok {
		t.Error("Close 后 Watch 应返回已关闭 channel")
	}
}

// Latch：仅首次 Trigger 生效，Done 关闭，HasFired 正确
func TestLatch(t *testing.T) {
	l := NewLatch()
	if l.HasFired() {
		t.Error("新建 Latch 不应已触发")
	}
	if !l.Trigger() {
		t.Error("首次 Trigger 应返回 true")
	}
	if !l.HasFired() {
		t.Error("触发后 HasFired 应为 true")
	}
	if l.Trigger() {
		t.Error("二次 Trigger 应返回 false")
	}
	select {
	case _, ok := <-l.Done():
		if ok {
			t.Error("Done 应已关闭")
		}
	default:
		t.Error("Done 应可读（已关闭）")
	}
}

// 多个等待方观察同一个 Latch
func TestLatch_MultipleWaiters(t *testing.T) {
	l := NewLatch()
	d1, d2 := l.Done(), l.Done()
	l.Trigger()
	for i, d := range []<-chan struct{}{d1, d2} {
		if _, ok := <-d; ok {
			t.Errorf("等待方 %d 的 Done 未关闭", i)
		}
	}
}

// deprecated 别名向后兼容
func TestDeprecatedAliases(t *testing.T) {
	if _, ok := any(NewOneEvent()).(*eventImpl); !ok {
		t.Error("NewOneEvent 别名应返回 eventImpl")
	}
	if !NewOnceEvent().Trigger() {
		t.Error("NewOnceEvent 别名 Trigger 应生效")
	}
}
