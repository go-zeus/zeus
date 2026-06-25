// Package memory 提供基于内存的进程内 pub/sub 实现。
//
// 设计目的：
//   - 零依赖（仅用标准库 + sync）
//   - 用作单进程内的事件总线（业务模块解耦）
//   - 用作单元测试的 mock broker
//
// 行为：
//   - 每个 topic 一个 channel（无缓冲，发布者阻塞直到所有订阅者读到）
//   - 每个 (topic, handler) 一个 goroutine（fan-out 给所有订阅者）
//   - Publish 阻塞直到所有 handler 接收（保证消息不丢，但慢消费者会阻塞发布者）
//   - Subscribe 后立即返回，handler 在独立 goroutine 处理
//   - Close 等待所有 in-flight handler 完成
//
// 局限：
//   - 不持久化（重启后丢失）
//   - 不支持多进程（不同 broker 实例独立，互不感知）
//   - 不保证消息顺序（多个订阅者各自 goroutine）
//   - 慢消费者会阻塞发布者（设计选择：保证消息不丢优于吞吐量）
//
// 高吞吐/跨进程场景请用 plugins/mq/kafka / nats / redis 等。
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/propagation"
)

// broker 内存消息代理
type broker struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscription // topic → subscriptions
	closed      bool
	errHandler  mq.ErrorHandler
	wg          sync.WaitGroup // 等待所有 handler goroutine 退出
}

// subscription 单个订阅（topic + handler + 消息 channel）
type subscription struct {
	topic   string
	handler mq.Handler
	ch      chan *mq.Message
	cancel  context.CancelFunc // 单个订阅的 ctx 取消
	done    chan struct{}      // handler goroutine 退出信号
}

// Option 配置 broker
type Option func(*broker)

// WithErrorHandler 注入自定义错误处理函数（默认 log.Error）
func WithErrorHandler(h mq.ErrorHandler) Option {
	return func(b *broker) {
		if h != nil {
			b.errHandler = h
		}
	}
}

// New 创建内存消息代理
func New(opts ...Option) mq.Broker {
	b := &broker{
		subscribers: make(map[string][]*subscription),
		errHandler:  defaultErrHandler,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

// defaultErrHandler 默认错误处理：log.Error
func defaultErrHandler(topic string, _ *mq.Message, err error) {
	log.Error("mq.memory: topic %s handler failed: %v", topic, err)
}

// Publish 发布消息
//
// 行为：
//   - 自动注入 ctx baggage 到 msg.Headers（与 propagation 集成）
//   - fan-out：把消息投递给所有该 topic 的订阅者
//   - 同步阻塞：所有订阅者的 channel 都接受后才返回（保证不丢）
//   - broker 已 Close 时返回 error
func (b *broker) Publish(ctx context.Context, topic string, msg *mq.Message) error {
	if err := mq.ValidateMessage(topic, msg); err != nil {
		return err
	}
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("mq.memory: broker closed")
	}
	// 自动注入 baggage（与 propagation 集成）
	if msg.Headers == nil {
		msg.Headers = make(map[string]string)
	}
	propagation.InjectMetadata(ctx, msg.Headers)
	// 回填 topic 便于 handler 直接读 msg.Topic
	msg.Topic = topic

	// 复制订阅列表避免 Publish 期间被修改
	subs := make([]*subscription, len(b.subscribers[topic]))
	copy(subs, b.subscribers[topic])
	b.mu.RUnlock()

	if len(subs) == 0 {
		// 无订阅者：丢弃（典型 pub/sub 语义）
		return nil
	}

	// fan-out：同步投递给每个订阅者的 channel
	// 注意：ch 是无缓冲 channel，发送会阻塞直到 handler goroutine 取走
	// 这是"慢消费者反压发布者"的设计选择
	for _, sub := range subs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sub.ch <- msg:
		}
	}
	return nil
}

// Subscribe 订阅 topic，handler 在后台 goroutine 调用
//
// 行为：
//   - 非阻塞：注册后立即返回
//   - 同 topic 多次 Subscribe：fan-out，每个 handler 独立 goroutine
//   - handler 在订阅 ctx 派生的新 ctx 下执行，ctx 自动注入 propagation baggage
//   - Close 时所有订阅 ctx 取消，handler 优雅退出
func (b *broker) Subscribe(ctx context.Context, topic string, handler mq.Handler) error {
	if topic == "" {
		return fmt.Errorf("mq.memory: topic is required")
	}
	if handler == nil {
		return fmt.Errorf("mq.memory: handler is nil")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("mq.memory: broker closed")
	}
	// 派生可取消 ctx：与 Subscribe 调用者的 ctx 解耦，Close 时能独立取消
	subCtx, cancel := context.WithCancel(ctx)
	sub := &subscription{
		topic:   topic,
		handler: handler,
		ch:      make(chan *mq.Message), // 无缓冲，反压
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	b.subscribers[topic] = append(b.subscribers[topic], sub)
	b.mu.Unlock()

	b.wg.Add(1)
	go b.runHandler(subCtx, sub)
	return nil
}

// runHandler 单个订阅者的消息处理 goroutine
func (b *broker) runHandler(ctx context.Context, sub *subscription) {
	defer b.wg.Done()
	defer close(sub.done)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sub.ch:
			// 自动 extract baggage 到 ctx（与 propagation 集成）
			handlerCtx := propagation.ExtractMetadata(ctx, msg.Headers)
			if err := sub.handler(handlerCtx, msg); err != nil {
				b.errHandler(sub.topic, msg, err)
			}
		}
	}
}

// Close 停止所有订阅，等待所有 in-flight handler 完成
//
// 行为：
//   - 取消所有订阅 ctx
//   - 等待所有 handler goroutine 退出
//   - 后续 Publish / Subscribe 返回 error
//   - 重复调用是 no-op
func (b *broker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	// 取消所有订阅
	for _, subs := range b.subscribers {
		for _, sub := range subs {
			sub.cancel()
		}
	}
	b.mu.Unlock()

	// 等待所有 handler 退出（无超时：in-flight handler 应自行通过 ctx 控制）
	b.wg.Wait()
	return nil
}
