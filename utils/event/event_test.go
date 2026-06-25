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
		// 成功收到事件
	case <-time.After(time.Second):
		t.Error("Watch() 后 Trigger，应在1秒内收到事件")
	}
}

// TestEvent_Unsubscribe 验证未 Watch 时 Trigger 不会阻塞，且不影响后续 Watch
func TestEvent_Unsubscribe(t *testing.T) {
	e := NewEvent()
	// 先触发，没有人 Watch
	e.Trigger()
	// 之后 Watch 应该能收到最新的事件（Trigger 会先清空旧事件再写入）
	ch := e.Watch()
	e.Trigger()
	select {
	case <-ch:
		// 成功收到事件
	case <-time.After(time.Second):
		t.Error("Trigger 后 Watch 应能收到事件")
	}
}

// TestEvent_Emit 验证 Trigger 后 Watch 通道可接收通知
func TestEvent_Emit(t *testing.T) {
	e := NewEvent()
	ch := e.Watch()
	e.Trigger()
	select {
	case <-ch:
		// 成功收到事件
	case <-time.After(time.Second):
		t.Error("Trigger 后应能通过 Watch 收到通知")
	}
}

// TestEvent_MultipleSubscribers 验证多个 Watch 各自独立通道，均能收到事件
// 修复后的 fan-out 语义：每个 watcher 拥有独立 channel，Trigger 时所有 watcher 都收到
func TestEvent_MultipleSubscribers(t *testing.T) {
	e := NewEvent()
	ch1 := e.Watch()
	ch2 := e.Watch()
	// 每个 Watch 返回独立 channel
	if ch1 == ch2 {
		t.Error("多次 Watch 应返回独立通道，避免订阅者之间抢事件")
	}
	e.Trigger()
	// 两个订阅者都应能收到事件（fan-out）
	for i, ch := range []<-chan struct{}{ch1, ch2} {
		select {
		case <-ch:
			// 订阅者成功收到
		case <-time.After(time.Second):
			t.Errorf("订阅者 %d 未能在1秒内收到事件", i+1)
		}
	}
}

// TestEvent_Close_ReleasesWatchers 验证 Close 关闭所有 watcher channel
func TestEvent_Close_ReleasesWatchers(t *testing.T) {
	e := NewEvent()
	ch1 := e.Watch()
	ch2 := e.Watch()
	e.Close()
	for i, ch := range []<-chan struct{}{ch1, ch2} {
		select {
		case <-ch:
			// Close 后 channel 应被关闭，接收方收到零值
		default:
			t.Errorf("订阅者 %d 的 channel 应被 Close 关闭", i+1)
		}
	}
	// Trigger 在 Close 后是 no-op
	e.Trigger() // 不应 panic
}
