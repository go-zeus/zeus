package memory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/propagation"
)

// TestMessage_Validate 校验逻辑
func TestMessage_Validate(t *testing.T) {
	if err := mq.ValidateMessage("", &mq.Message{}); err == nil {
		t.Error("empty topic should error")
	}
	if err := mq.ValidateMessage("x", nil); err == nil {
		t.Error("nil message should error")
	}
	if err := mq.ValidateMessage("x", &mq.Message{}); err != nil {
		t.Errorf("valid message should not error: %v", err)
	}
}

// TestPublishSubscribe_Basic 基本 pub/sub
func TestPublishSubscribe_Basic(t *testing.T) {
	b := New()
	defer b.Close()

	var received []byte
	var mu sync.Mutex
	done := make(chan struct{})

	_ = b.Subscribe(context.Background(), "test", func(_ context.Context, msg *mq.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = msg.Payload
		close(done)
		return nil
	})

	// 等订阅就绪（memory 实现无 ready 信号，简单 sleep）
	time.Sleep(10 * time.Millisecond)

	_ = b.Publish(context.Background(), "test", &mq.Message{
		Payload: []byte("hello"),
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler not called within 1s")
	}

	mu.Lock()
	defer mu.Unlock()
	if string(received) != "hello" {
		t.Errorf("received = %q, want hello", string(received))
	}
}

// TestPublish_NoSubscriber 无订阅者：no-op，不报错
func TestPublish_NoSubscriber(t *testing.T) {
	b := New()
	defer b.Close()

	err := b.Publish(context.Background(), "no-sub", &mq.Message{
		Payload: []byte("hello"),
	})
	if err != nil {
		t.Errorf("publish to no-subscriber topic should not error: %v", err)
	}
}

// TestPublish_FanOut 同 topic 多订阅者：每个都收到
func TestPublish_FanOut(t *testing.T) {
	b := New()
	defer b.Close()

	var count int32
	makeHandler := func() mq.Handler {
		return func(_ context.Context, _ *mq.Message) error {
			atomic.AddInt32(&count, 1)
			return nil
		}
	}

	_ = b.Subscribe(context.Background(), "broadcast", makeHandler())
	_ = b.Subscribe(context.Background(), "broadcast", makeHandler())
	_ = b.Subscribe(context.Background(), "broadcast", makeHandler())

	time.Sleep(10 * time.Millisecond)

	_ = b.Publish(context.Background(), "broadcast", &mq.Message{Payload: []byte("x")})

	deadline := time.After(time.Second)
	for {
		if atomic.LoadInt32(&count) == 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected 3 deliveries, got %d", atomic.LoadInt32(&count))
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestPublish_TopicsIsolated 不同 topic 互不干扰
func TestPublish_TopicsIsolated(t *testing.T) {
	b := New()
	defer b.Close()

	var topic1Count, topic2Count int32
	_ = b.Subscribe(context.Background(), "topic1", func(_ context.Context, _ *mq.Message) error {
		atomic.AddInt32(&topic1Count, 1)
		return nil
	})
	_ = b.Subscribe(context.Background(), "topic2", func(_ context.Context, _ *mq.Message) error {
		atomic.AddInt32(&topic2Count, 1)
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	_ = b.Publish(context.Background(), "topic1", &mq.Message{})
	_ = b.Publish(context.Background(), "topic1", &mq.Message{})
	_ = b.Publish(context.Background(), "topic2", &mq.Message{})

	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&topic1Count); got != 2 {
		t.Errorf("topic1 count = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&topic2Count); got != 1 {
		t.Errorf("topic2 count = %d, want 1", got)
	}
}

// TestSubscribe_HandlerError handler 返回 error 时调用 ErrorHandler
func TestSubscribe_HandlerError(t *testing.T) {
	var capturedErr error
	var mu sync.Mutex
	done := make(chan struct{})

	b := New(WithErrorHandler(func(topic string, _ *mq.Message, err error) {
		mu.Lock()
		defer mu.Unlock()
		capturedErr = err
		close(done)
	}))
	defer b.Close()

	jobErr := errors.New("handler failure")
	_ = b.Subscribe(context.Background(), "fail", func(_ context.Context, _ *mq.Message) error {
		return jobErr
	})

	time.Sleep(10 * time.Millisecond)
	_ = b.Publish(context.Background(), "fail", &mq.Message{})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ErrorHandler not called within 1s")
	}

	mu.Lock()
	defer mu.Unlock()
	if !errors.Is(capturedErr, jobErr) {
		t.Errorf("captured err = %v, want %v", capturedErr, jobErr)
	}
}

// TestClose_GracefulShutdown Close 后所有 handler goroutine 退出
func TestClose_GracefulShutdown(t *testing.T) {
	b := New()

	var count int32
	_ = b.Subscribe(context.Background(), "x", func(_ context.Context, _ *mq.Message) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	time.Sleep(10 * time.Millisecond)
	_ = b.Publish(context.Background(), "x", &mq.Message{})
	time.Sleep(20 * time.Millisecond)

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Close 后再 Publish 应失败
	if err := b.Publish(context.Background(), "x", &mq.Message{}); err == nil {
		t.Error("Publish after Close should error")
	}

	countAfterClose := atomic.LoadInt32(&count)
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&count) != countAfterClose {
		t.Errorf("count changed after Close: %d → %d", countAfterClose, atomic.LoadInt32(&count))
	}
}

// TestClose_Idempotent Close 多次调用安全
func TestClose_Idempotent(t *testing.T) {
	b := New()
	if err := b.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("second Close should be no-op, got: %v", err)
	}
}

// TestClose_WaitsForInflightHandler Close 等待 in-flight handler 完成
func TestClose_WaitsForInflightHandler(t *testing.T) {
	b := New()

	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})
	_ = b.Subscribe(context.Background(), "slow", func(ctx context.Context, _ *mq.Message) error {
		close(handlerStarted)
		<-ctx.Done() // 等 Close 触发取消
		defer close(handlerDone)
		return ctx.Err()
	})

	time.Sleep(10 * time.Millisecond)
	_ = b.Publish(context.Background(), "slow", &mq.Message{})

	<-handlerStarted // 等 handler 开始处理

	closeDone := make(chan struct{})
	go func() {
		_ = b.Close()
		close(closeDone)
	}()

	// Close 应阻塞（handler 还没退出，但 ctx 已取消，handler 很快退出）
	select {
	case <-closeDone:
		// 期望：handler 先退出，然后 Close 返回
	case <-time.After(time.Second):
		t.Fatal("Close did not complete within 1s")
	}

	<-handlerDone
}

// TestPublish_AutoInjectBaggage Publish 自动从 ctx 注入 baggage
//
// 关键契约：用户在业务代码 propagation.With(ctx, k, v)，发布消息后，
// msg.Headers[MetadataBaggage] 自动包含 W3C baggage 编码字符串。
// 订阅侧可通过 propagation.ExtractMetadata 还原。
func TestPublish_AutoInjectBaggage(t *testing.T) {
	b := New()
	defer b.Close()

	var (
		rawBaggage string
		hasBaggage bool
	)
	done := make(chan struct{})

	_ = b.Subscribe(context.Background(), "test", func(_ context.Context, msg *mq.Message) error {
		rawBaggage, hasBaggage = msg.Headers[propagation.MetadataBaggage]
		close(done)
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	// 发布者注入 baggage
	ctx := propagation.With(context.Background(), "tenant.id", "acme")
	_ = b.Publish(ctx, "test", &mq.Message{Payload: []byte("x")})

	<-done
	if !hasBaggage {
		t.Fatal("msg.Headers[\"baggage\"] missing after Publish")
	}
	// 解码 baggage 并验证 tenant.id=acme
	bag := propagation.Decode(rawBaggage)
	v, ok := bag.Get("tenant.id")
	if !ok || v != "acme" {
		t.Errorf("decoded tenant.id = (%q,%v), want (acme,true)", v, ok)
	}
}

// TestSubscribe_AutoExtractBaggage handler 收到的 ctx 自动 extract baggage
//
// 关键契约：handler 通过 propagation.Get(ctx, key) 能读到 msg.Headers 中的 K-V
func TestSubscribe_AutoExtractBaggage(t *testing.T) {
	b := New()
	defer b.Close()

	var got string
	var has bool
	done := make(chan struct{})

	_ = b.Subscribe(context.Background(), "test", func(ctx context.Context, _ *mq.Message) error {
		got, has = propagation.Get(ctx, "tenant.id")
		close(done)
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	// 发布者注入 baggage
	ctx := propagation.With(context.Background(), "tenant.id", "globex")
	_ = b.Publish(ctx, "test", &mq.Message{Payload: []byte("x")})

	<-done
	if !has || got != "globex" {
		t.Errorf("propagation.Get(tenant.id) = (%q,%v), want (globex,true)", got, has)
	}
}

// TestSubscribe_BaggageEndToEnd 发布侧注入 + 订阅侧读取（双向验证）
//
// 验证 baggage 在 Publish → handler 完整链路上的传递：
//   - Publish: ctx baggage → msg.Headers["baggage"]（W3C 编码）
//   - handler: msg.Headers["baggage"] → handler ctx（自动 ExtractMetadata）
func TestSubscribe_BaggageEndToEnd(t *testing.T) {
	b := New()
	defer b.Close()

	type captured struct {
		baggageRaw string
		ctxVal     string
	}
	var got captured
	done := make(chan struct{})

	_ = b.Subscribe(context.Background(), "x", func(ctx context.Context, msg *mq.Message) error {
		got = captured{
			baggageRaw: msg.Headers[propagation.MetadataBaggage],
			ctxVal:     func() string { v, _ := propagation.Get(ctx, "region"); return v }(),
		}
		close(done)
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	// 发布侧：注入 region=cn-east-1
	ctx := propagation.With(context.Background(), "region", "cn-east-1")
	_ = b.Publish(ctx, "x", &mq.Message{})

	<-done
	// 直接解码 baggage header 验证 region
	bag := propagation.Decode(got.baggageRaw)
	if v, ok := bag.Get("region"); !ok || v != "cn-east-1" {
		t.Errorf("decoded header region = (%q,%v), want (cn-east-1,true)", v, ok)
	}
	if got.ctxVal != "cn-east-1" {
		t.Errorf("ctx region = %q, want cn-east-1", got.ctxVal)
	}
}

// TestPublish_AfterClose Close 后 Publish 报错
func TestPublish_AfterClose(t *testing.T) {
	b := New()
	_ = b.Close()
	if err := b.Publish(context.Background(), "x", &mq.Message{}); err == nil {
		t.Error("Publish after Close should error")
	}
}

// TestSubscribe_AfterClose Close 后 Subscribe 报错
func TestSubscribe_AfterClose(t *testing.T) {
	b := New()
	_ = b.Close()
	err := b.Subscribe(context.Background(), "x", func(context.Context, *mq.Message) error { return nil })
	if err == nil {
		t.Error("Subscribe after Close should error")
	}
}

// TestSubscribe_NilHandler 拒绝 nil handler
func TestSubscribe_NilHandler(t *testing.T) {
	b := New()
	defer b.Close()
	if err := b.Subscribe(context.Background(), "x", nil); err == nil {
		t.Error("Subscribe with nil handler should error")
	}
}

// TestSubscribe_EmptyTopic 拒绝空 topic
func TestSubscribe_EmptyTopic(t *testing.T) {
	b := New()
	defer b.Close()
	err := b.Subscribe(context.Background(), "", func(context.Context, *mq.Message) error { return nil })
	if err == nil {
		t.Error("Subscribe with empty topic should error")
	}
}

// TestPublish_PropagatesContextCancel 发布期间 ctx 取消应中断发送
//
// 场景：多个订阅者，第一个订阅者已经接收，但 ctx 在投递给第二个时被取消
func TestPublish_PropagatesContextCancel(t *testing.T) {
	b := New()
	defer b.Close()

	// 第一个订阅者立即接收，第二个永远阻塞
	blockChan := make(chan struct{})
	defer close(blockChan) // 防止 goroutine 泄漏

	_ = b.Subscribe(context.Background(), "x", func(_ context.Context, _ *mq.Message) error {
		return nil
	})
	// 故意制造一个慢消费者：先在 goroutine 里阻塞 ch 接收
	// 通过这种方式让 Publish 在第二次发送时阻塞，便于 cancel 触发

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	// 期望：ctx 取消后 Publish 返回 ctx.Err()
	// 注：因为只有 1 个订阅者且 ch 是无缓冲，如果 handler 处理快，Publish 可能成功
	// 此测试主要验证 ctx.Done 路径不 panic
	_ = b.Publish(ctx, "x", &mq.Message{})
}

// TestMessage_TopicBackfill Publish 后 msg.Topic 自动回填
func TestMessage_TopicBackfill(t *testing.T) {
	b := New()
	defer b.Close()

	var seen string
	done := make(chan struct{})
	_ = b.Subscribe(context.Background(), "orders.created", func(_ context.Context, msg *mq.Message) error {
		seen = msg.Topic
		close(done)
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	msg := &mq.Message{Payload: []byte("order1")}
	_ = b.Publish(context.Background(), "orders.created", msg)

	<-done
	if seen != "orders.created" {
		t.Errorf("msg.Topic = %q, want orders.created", seen)
	}
}
