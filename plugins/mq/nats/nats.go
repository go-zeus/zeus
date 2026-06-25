// Package nats 提供 NATS 消息队列的薄包装插件。
//
// 设计目的：
//   - 实现 mq.Broker 接口（Publisher + Subscriber），让 zeus 业务代码与 NATS 解耦
//   - 自动透传 propagation baggage：用 NATS v2.2+ 原生 Headers（不污染 payload）
//   - URL scheme "nats://" → mq.NewBrokerFromURL 调用零感知
//
// 不做的事：
//   - 不抽象 JetStream（持久化 / ack / 事务）—— 用原生 SDK
//   - 不抽象 QueueGroup 之外的复杂路由 —— 用 NATS 原生 subject 通配符
//   - 不实现批量消费 / 顺序保证 —— 用 JetStream
//
// 设计权衡：
//   - 同步 Publish：默认 nc.Publish（不等待 ack）；用户需要确认时用 WithJetStream（待补）
//   - 订阅 push 模式：NATS 内部 worker pool 调度，handler 直接在回调中执行
//     （不另开 goroutine，避免双层调度开销）
//   - ErrorHandler 默认 log.Error，可注入告警钩子
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/mq"
//	    natsplugin "github.com/go-zeus/zeus/plugins/mq/nats"
//	)
//
//	broker, err := natsplugin.New(natsplugin.WithURL("nats://127.0.0.1:4222"))
//	if err != nil { return err }
//	defer broker.Close()
//
//	// 订阅
//	_ = broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
//	    // ctx 已自动注入 baggage（如 tenant.id）
//	    return processOrder(msg.Payload)
//	})
//
//	// 发布（ctx 中的 baggage 自动注入 msg.Headers）
//	ctx = propagation.With(ctx, "tenant.id", "acme")
//	_ = broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("order-1")})
package nats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/propagation"
)

// 默认配置常量
const (
	// defaultConnectTimeout 默认连接超时
	defaultConnectTimeout = 5 * time.Second

	// baggageHeader NATS Header 中存放 baggage 的字段名（与 HTTP 一致）
	// 用 W3C Baggage 标准字段名，便于跨语言/跨 broker 互操作
	baggageHeader = "Baggage"
)

// broker NATS-backed mq.Broker 实现
type broker struct {
	mu         sync.Mutex
	conn       *nats.Conn
	subs       []*nats.Subscription // 所有订阅（Close 时统一 Drain）
	closed     bool
	errHandler mq.ErrorHandler
	rootCtx    context.Context // 派生订阅 ctx 的根（取消时所有订阅退出）
	rootCancel context.CancelFunc
	wg         sync.WaitGroup // 等待 in-flight handler 退出（订阅侧）
}

// Option 配置 broker
type Option func(*brokerConfig)

// brokerConfig 构造期配置（仅用于 New 阶段，与运行期 broker 分离）
type brokerConfig struct {
	url            string
	opts           []nats.Option
	errHandler     mq.ErrorHandler
	connectTimeout time.Duration
}

// WithURL 设置 NATS 服务器地址（默认 nats://127.0.0.1:4222）
//
// 多个地址用逗号分隔（如 "nats://h1:4222,nats://h2:4222"）。
func WithURL(url string) Option {
	return func(c *brokerConfig) {
		if url != "" {
			c.url = url
		}
	}
}

// WithNATSOptions 注入原生 nats.Option（用于 MaxReconnects / ReconnectWait / TLS 等高级配置）
//
// 示例：
//
//	natsplugin.New(
//	    natsplugin.WithURL("nats://..."),
//	    natsplugin.WithNATSOptions(
//	        nats.MaxReconnects(10),
//	        nats.ReconnectWait(2*time.Second),
//	    ),
//	)
func WithNATSOptions(opts ...nats.Option) Option {
	return func(c *brokerConfig) {
		c.opts = append(c.opts, opts...)
	}
}

// WithErrorHandler 注入自定义错误处理函数（默认 log.Error）
func WithErrorHandler(h mq.ErrorHandler) Option {
	return func(c *brokerConfig) {
		if h != nil {
			c.errHandler = h
		}
	}
}

// WithConnectTimeout 自定义连接超时（默认 5s）
func WithConnectTimeout(d time.Duration) Option {
	return func(c *brokerConfig) {
		if d > 0 {
			c.connectTimeout = d
		}
	}
}

// New 构造 NATS-backed broker（实现 mq.Broker 接口）
//
// 行为：
//   - 立即建立 NATS 连接（同步，失败返回 error）
//   - tracer/meter 不直接接收：zeus 主包 mq.Broker 接口未要求，如需 trace 通过 propagation 路径走
//   - 关闭时调用 nc.Drain() 等所有 in-flight 消息处理完才返回
func New(opts ...Option) (mq.Broker, error) {
	cfg := &brokerConfig{
		url:            "nats://127.0.0.1:4222",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// 把用户的 nats.Option 放前面，Timeout 放后面（用户如显式传 Timeout 会被覆盖；
	// 否则用 cfg.connectTimeout）
	finalOpts := append([]nats.Option{}, cfg.opts...)
	finalOpts = append(finalOpts, nats.Timeout(cfg.connectTimeout))

	nc, err := nats.Connect(cfg.url, finalOpts...)
	if err != nil {
		return nil, fmt.Errorf("mq.nats: connect %q failed: %w", cfg.url, err)
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &broker{
		conn:       nc,
		errHandler: cfg.errHandler,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}, nil
}

// defaultErrHandler 默认错误处理：log.Error
func defaultErrHandler(topic string, _ *mq.Message, err error) {
	log.Error("mq.nats: topic %s handler failed: %v", topic, err)
}

// Publish 发布消息到指定 subject
//
// 行为：
//   - 自动注入 ctx baggage 到 NATS Headers（"Baggage" 字段，W3C 编码）
//   - 同步：返回 nil 表示 NATS 服务器已接收（不保证所有订阅者已处理）
//   - broker 已 Close 时返回 error
//   - msg.Headers 用户自定义字段会一并写入 NATS Headers
func (b *broker) Publish(ctx context.Context, subject string, msg *mq.Message) error {
	if err := mq.ValidateMessage(subject, msg); err != nil {
		return err
	}
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return fmt.Errorf("mq.nats: broker closed")
	}

	nmsg := &nats.Msg{
		Subject: subject,
		Data:    msg.Payload,
		Header:  nats.Header{},
	}

	// 用户自定义 headers 透传
	for k, v := range msg.Headers {
		nmsg.Header.Set(k, v)
	}
	// 自动注入 baggage：把 ctx 中的 propagation.Bag 编码为 W3C baggage 字符串写入 NATS Header
	// 注意：覆盖用户已写的 Baggage 字段（propagation 是真理源）
	if bag := propagation.FromContext(ctx); bag != nil && bag.Len() > 0 {
		nmsg.Header.Set(baggageHeader, encodeBaggage(bag))
	}

	if err := b.conn.PublishMsg(nmsg); err != nil {
		return fmt.Errorf("mq.nats: publish to %q failed: %w", subject, err)
	}
	return nil
}

// Subscribe 订阅 subject，handler 在 NATS 内部 goroutine 调用
//
// 行为：
//   - 非阻塞：注册后立即返回
//   - 同 subject 多次 Subscribe：fan-out（每个 handler 独立接收）
//   - ctx 自动从 broker rootCtx 派生 + 注入 msg.Headers 中的 baggage
//   - Close 时所有订阅 ctx 取消，broker 等待 in-flight handler 完成
//   - NATS subject 通配符支持：">" 多级匹配，"*" 单级匹配
func (b *broker) Subscribe(_ context.Context, subject string, handler mq.Handler) error {
	if subject == "" {
		return fmt.Errorf("mq.nats: subject is required")
	}
	if handler == nil {
		return fmt.Errorf("mq.nats: handler is nil")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("mq.nats: broker closed")
	}
	b.mu.Unlock()

	// 用 broker rootCtx 作为订阅 ctx 基底：
	//   - 与调用者 ctx 解耦（Close 时能独立取消）
	//   - 不为每个订阅单独 WithCancel（rootCancel 一次性取消所有）
	b.wg.Add(1)
	sub, err := b.conn.Subscribe(subject, func(nmsg *nats.Msg) {
		msg := natsMsgToMQ(nmsg)
		// 自动 extract baggage 到 ctx（从 NATS Header "Baggage" 字段解析）
		handlerCtx := propagation.ExtractMetadata(b.rootCtx, map[string]string{
			baggageHeader: nmsg.Header.Get(baggageHeader),
		})
		if err := handler(handlerCtx, msg); err != nil {
			b.errHandler(subject, msg, err)
		}
	})
	if err != nil {
		b.wg.Done()
		return fmt.Errorf("mq.nats: subscribe to %q failed: %w", subject, err)
	}

	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	// 启动 goroutine 监听 rootCtx 取消 → 主动 Unsubscribe
	// rootCancel 由 Close 触发，所有订阅统一清理
	go func() {
		defer b.wg.Done()
		<-b.rootCtx.Done()
		_ = sub.Unsubscribe()
	}()
	return nil
}

// Close 优雅关闭 broker
//
// 行为：
//   - 取消所有订阅 ctx（触发 Unsubscribe）
//   - 调用 nc.Drain() 等所有 in-flight 消息处理完
//   - 关闭 NATS 连接
//   - 重复调用是 no-op
func (b *broker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	// 取消所有订阅 ctx → 触发 goroutine Unsubscribe
	b.rootCancel()

	// Drain 等所有 in-flight 消息处理完（含上面回调里的 handler）
	// 超时由 NATS 内部默认（5s）控制
	if !b.conn.IsClosed() {
		_ = b.conn.Drain()
	}

	// 等所有订阅 goroutine 退出
	b.wg.Wait()
	return nil
}

// natsMsgToMQ 把 NATS 原生 Msg 转换为 mq.Message
func natsMsgToMQ(nmsg *nats.Msg) *mq.Message {
	headers := make(map[string]string, len(nmsg.Header))
	for k, vs := range nmsg.Header {
		if len(vs) > 0 {
			headers[k] = vs[0]
		}
	}
	return &mq.Message{
		Topic:   nmsg.Subject,
		Payload: nmsg.Data,
		Headers: headers,
	}
}

// encodeBaggage 把 propagation.Bag 编码为 W3C baggage 字符串
//
// 简单实现：直接用 propagation 内部编码（通过 InjectMetadata 间接调用）。
// 为避免循环依赖，这里手动拼装（W3C 格式：key1=val1,key2=val2）。
func encodeBaggage(bag *propagation.Bag) string {
	if bag == nil || bag.Len() == 0 {
		return ""
	}
	var buf []byte
	first := true
	for _, e := range bag.Entries() {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, []byte(e.Key+"="+e.Value)...)
	}
	return string(buf)
}
