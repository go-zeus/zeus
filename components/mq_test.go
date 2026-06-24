package components

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/mq/memory"
)

// mockBroker 用于测试 MQComponent 是否正确编排
type mockBroker struct {
	mu            sync.Mutex
	subscriptions []mockSub
	closed        bool
	startSubErr   error
	closeErr      error
}

type mockSub struct {
	topic   string
	handler mq.Handler
}

func (m *mockBroker) Publish(_ context.Context, _ string, _ *mq.Message) error {
	return nil
}

func (m *mockBroker) Subscribe(_ context.Context, topic string, handler mq.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startSubErr != nil {
		return m.startSubErr
	}
	m.subscriptions = append(m.subscriptions, mockSub{topic: topic, handler: handler})
	return nil
}

func (m *mockBroker) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closeErr != nil {
		return m.closeErr
	}
	m.closed = true
	return nil
}

// TestMQComponent_Lifecycle MQComponent.OnStart 收集 + 注册；OnStop 关闭
func TestMQComponent_Lifecycle(t *testing.T) {
	mock := &mockBroker{}
	mc := NewMQComponent(mock)

	sub1 := NewMQSubscription("topic.a", func(context.Context, *mq.Message) error { return nil })
	sub2 := NewMQSubscription("topic.b", func(context.Context, *mq.Message) error { return nil })

	c := NewContainer()
	_ = c.Register(mc)
	_ = c.Register(sub1)
	_ = c.Register(sub2)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Container.Start: %v", err)
	}

	mock.mu.Lock()
	if len(mock.subscriptions) != 2 {
		t.Errorf("registered %d subs, want 2", len(mock.subscriptions))
	}
	topics := map[string]bool{}
	for _, s := range mock.subscriptions {
		topics[s.topic] = true
	}
	if !topics["topic.a"] || !topics["topic.b"] {
		t.Errorf("topic set = %v, want both topic.a and topic.b", topics)
	}
	mock.mu.Unlock()

	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Container.Stop: %v", err)
	}
	if !mock.closed {
		t.Error("Broker.Close should be called on stop")
	}
}

// TestMQComponent_Integration 端到端：Container 启动后发布/订阅链路通
//
// 业务代码取 broker 的两种方式：
//  1. 测试 / 同进程：注册时直接持有 broker 引用（本测试用法）
//  2. 跨组件：在 OnStart 钩子里通过 Type[mq.Broker](ctx) 取（见 TestMQComponent_GetByType）
func TestMQComponent_Integration(t *testing.T) {
	var received []byte
	var mu sync.Mutex
	done := make(chan struct{})

	broker := memory.New()
	mc := NewMQComponent(broker)
	sub := NewMQSubscription("test.topic", func(_ context.Context, msg *mq.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = msg.Payload
		close(done)
		return nil
	})

	c := NewContainer()
	_ = c.Register(mc)
	_ = c.Register(sub)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Container.Start: %v", err)
	}

	// 等订阅就绪
	time.Sleep(20 * time.Millisecond)

	// 通过 broker 引用发布（业务代码同理：注册时持有 broker 即可）
	if err := broker.Publish(ctx, "test.topic", &mq.Message{Payload: []byte("hello")}); err != nil {
		t.Fatalf("publish: %v", err)
	}

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

	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Container.Stop: %v", err)
	}
}

// TestMQComponent_NoBroker Broker 为 nil 时 no-op
func TestMQComponent_NoBroker(t *testing.T) {
	mc := NewMQComponent(nil)
	c := NewContainer()
	_ = c.Register(mc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start with nil broker should be no-op, got: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop with nil broker should be no-op, got: %v", err)
	}
}

// TestMQComponent_SubscribeError Broker.Subscribe 失败时 Container.Start 返回 error
func TestMQComponent_SubscribeError(t *testing.T) {
	subErr := errors.New("subscribe failed")
	mock := &mockBroker{startSubErr: subErr}
	mc := NewMQComponent(mock)
	sub := NewMQSubscription("x", func(context.Context, *mq.Message) error { return nil })

	c := NewContainer()
	_ = c.Register(mc)
	_ = c.Register(sub)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Broker.Subscribe")
	}
	if !errors.Is(err, subErr) {
		t.Errorf("err = %v, want %v", err, subErr)
	}
}

// TestMQComponent_FanOut 同 topic 多订阅者：每个都收到（通过 GetAllByType 收集）
func TestMQComponent_FanOut(t *testing.T) {
	var count int32
	broker := memory.New()
	mc := NewMQComponent(broker)

	// 同 topic 多个 MQSubscription：每个应被独立收集 + 订阅
	h := func(context.Context, *mq.Message) error {
		atomic.AddInt32(&count, 1)
		return nil
	}

	c := NewContainer()
	_ = c.Register(mc)
	_ = c.Register(NewMQSubscription("broadcast", h))
	_ = c.Register(NewMQSubscription("broadcast", h))
	_ = c.Register(NewMQSubscription("broadcast", h))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	_ = broker.Publish(ctx, "broadcast", &mq.Message{Payload: []byte("x")})

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

	_ = c.Stop(ctx)
}

// TestMQSubscription_Name 包含 topic + 唯一序号用于诊断
func TestMQSubscription_Name(t *testing.T) {
	s := NewMQSubscription("orders.created", func(context.Context, *mq.Message) error { return nil })
	prefix := "mq_subscription:orders.created:"
	if len(s.Name()) <= len(prefix) || s.Name()[:len(prefix)] != prefix {
		t.Errorf("Name = %q, want prefix %q", s.Name(), prefix)
	}
}

// TestMQSubscription_NameUnique 同 topic 多次注册时 Name 不重复
func TestMQSubscription_NameUnique(t *testing.T) {
	s1 := NewMQSubscription("x", func(context.Context, *mq.Message) error { return nil })
	s2 := NewMQSubscription("x", func(context.Context, *mq.Message) error { return nil })
	if s1.Name() == s2.Name() {
		t.Errorf("two subscriptions with same topic should have different Name, got %q twice", s1.Name())
	}
}

// TestMQSubscription_DependsOnMQ 依赖 mq 组件（保证 Broker 先就绪）
func TestMQSubscription_DependsOnMQ(t *testing.T) {
	s := NewMQSubscription("x", func(context.Context, *mq.Message) error { return nil })
	deps := s.Depends()
	if len(deps) != 1 || deps[0] != "mq" {
		t.Errorf("Depends = %v, want [mq]", deps)
	}
}

// TestMQComponent_GetByType OnStart 内可通过 Type[mq.Broker] 获取 broker
//
// 验证 Provide 把 broker 发布到容器，类型为 mq.Broker
func TestMQComponent_GetByType(t *testing.T) {
	broker := memory.New()
	mc := NewMQComponent(broker)

	captured := make(chan mq.Broker, 1)
	c := NewContainer()
	_ = c.Register(mc)
	// 用一个 OnStart 钩子抓取 broker
	_ = c.Register(&brokerCapturer{capture: captured})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case got := <-captured:
		if got == nil {
			t.Error("captured broker is nil")
		}
	case <-time.After(time.Second):
		t.Fatal("broker not captured within 1s")
	}

	_ = c.Stop(ctx)
}

// brokerCapturer 测试辅助组件：OnStart 时通过 Type[mq.Broker] 抓取 broker
type brokerCapturer struct {
	capture chan<- mq.Broker
}

func (b *brokerCapturer) Name() string      { return "broker_capturer" }
func (b *brokerCapturer) Depends() []string { return []string{"mq"} }
func (b *brokerCapturer) Provide(_ Context) (any, error) {
	return b, nil
}
func (b *brokerCapturer) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			br, err := Type[mq.Broker](ctx)
			if err != nil {
				return err
			}
			b.capture <- br
			return nil
		},
	}
}
