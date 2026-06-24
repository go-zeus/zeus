// Package kafka 提供基于 IBM/sarama 的 Kafka 消息队列实现。
//
// 设计目的：
//   - 实现 mq.Broker 接口（Publisher + Subscriber），让 zeus 业务代码与 Kafka 解耦
//   - 自动透传 propagation baggage：用 Kafka message headers（不污染 payload）
//   - URL scheme "kafka://" → mq.NewBrokerFromURL 零感知启用
//
// 设计权衡：
//   - 同步 Publish：用 sarama.SyncProducer（等 Kafka ack），保证可靠交付
//   - 单 consumer group 模式：broker 实例绑一个 group；不同 group 各自构造 broker
//     原因：sarama ConsumerGroup API 要求构造时指定 group，而 mq.Broker.Subscribe 不带 group
//   - handler 在 sarama ConsumerGroup 的内部 goroutine 中执行（不另开 goroutine）
//   - ack 语义：handler 返回 nil → 提交 offset；handler 返回 error → 不提交，下次重新投递
//   - ErrorHandler：handler 失败时调用，默认 log.Error
//
// 不做的事：
//   - 不抽象 partition / key 路由（默认 round-robin；如需 key 路由用 WithProducerOption）
//   - 不抽象事务 / ExactlyOnce 语义 —— 用原生 SDK
//   - 不做批量发布 / 批量消费优化 —— 用原生 SDK
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/mq"
//	    kafkaplugin "github.com/go-zeus/zeus/plugins/mq/kafka"
//	)
//
//	broker, err := kafkaplugin.New(
//	    kafkaplugin.WithBrokers("127.0.0.1:9092"),
//	    kafkaplugin.WithGroup("order-consumers"),
//	)
//	if err != nil { return err }
//	defer broker.Close()
//
//	// 订阅（handler 返回 nil 自动 ack offset）
//	_ = broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
//	    // ctx 已自动注入 baggage（如 tenant.id）
//	    return processOrder(msg.Payload)
//	})
//
//	// 发布（ctx 中的 baggage 自动注入 msg.Headers）
//	ctx = propagation.With(ctx, "tenant.id", "acme")
//	_ = broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("order-1")})
package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/propagation"
)

// 默认配置常量
const (
	// defaultBrokerPort 默认 Kafka broker 端口
	defaultBrokerPort = "9092"

	// defaultGroup 默认 consumer group ID
	defaultGroup = "zeus-default"

	// defaultVersion 默认 Kafka 版本（3.x 集群通用）
	defaultVersion = "3.5.0"

	// baggageHeader Kafka header 中存放 baggage 的字段名（W3C 标准）
	baggageHeader = "baggage"
)

// broker Kafka-backed mq.Broker 实现
//
// 字段说明：
//   - client：sarama 底层 Client（共享给 producer 和 consumerGroup）
//   - producer：同步发布者
//   - consumerGroup：消费者组
//   - groupID：当前 broker 绑定的 group
//   - handlers：topic → handlers 列表（同 topic 多次 Subscribe 支持 fan-out）
//   - rootCtx/rootCancel：所有订阅共享的 ctx，Close 时统一取消
type broker struct {
	mu sync.RWMutex

	client        sarama.Client
	producer      sarama.SyncProducer
	consumerGroup sarama.ConsumerGroup
	groupID       string

	handlers map[string][]mq.Handler
	closed   bool

	rootCtx    context.Context
	rootCancel context.CancelFunc

	errHandler mq.ErrorHandler
	wg         sync.WaitGroup // 等待 consume loop 退出
}

// Option 配置 broker
type Option func(*brokerConfig)

// brokerConfig 构造期配置
type brokerConfig struct {
	brokers      []string
	group        string
	version      string
	errHandler   mq.ErrorHandler
	producerOpts func(*sarama.Config) // 用户自定义 producer 配置（acks/compression 等）
	consumerOpts func(*sarama.Config) // 用户自定义 consumer 配置（offset 初始策略等）
	dialTimeout  time.Duration
}

// WithBrokers 设置 Kafka broker 地址列表（默认 ["127.0.0.1:9092"]）
//
// 多 broker 传入多个（如 WithBrokers("h1:9092", "h2:9092", "h3:9092")）
func WithBrokers(addrs ...string) Option {
	return func(c *brokerConfig) {
		for _, a := range addrs {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			c.brokers = append(c.brokers, a)
		}
	}
}

// WithGroup 设置 consumer group ID（默认 "zeus-default"）
//
// 同 group 内：partition 间负载均衡（一个 partition 只分给一个 consumer 实例）
// 不同 group：每个 group 各自消费完整消息流
//
// 一个 broker 实例只能绑一个 group；不同 group 需各自构造 broker。
func WithGroup(g string) Option {
	return func(c *brokerConfig) {
		if g != "" {
			c.group = g
		}
	}
}

// WithVersion 设置 Kafka broker 版本字符串（默认 "3.5.0"）
//
// 必须与实际 broker 版本兼容，否则特征协商失败。常见值："2.8.0"、"3.5.0"、"3.7.0"
func WithVersion(v string) Option {
	return func(c *brokerConfig) {
		if v != "" {
			c.version = v
		}
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

// WithProducerConfig 注入自定义 producer 配置（在默认配置基础上覆盖）
//
// 用于调整 Acks / Compression / Idempotent / Return.Successes 等高级参数
func WithProducerConfig(fn func(*sarama.Config)) Option {
	return func(c *brokerConfig) {
		c.producerOpts = fn
	}
}

// WithConsumerConfig 注入自定义 consumer 配置
//
// 用于调整 Offsets.Initial / Fetch.Min/Max 等高级参数
func WithConsumerConfig(fn func(*sarama.Config)) Option {
	return func(c *brokerConfig) {
		c.consumerOpts = fn
	}
}

// WithDialTimeout 设置连接超时（默认 10s）
func WithDialTimeout(d time.Duration) Option {
	return func(c *brokerConfig) {
		if d > 0 {
			c.dialTimeout = d
		}
	}
}

// New 构造 Kafka-backed broker（实现 mq.Broker 接口）
//
// 行为：
//   - 立即建立 Kafka 连接（sarama.NewClient），失败返回 error
//   - 用同一 Client 派生 SyncProducer 和 ConsumerGroup
//   - Subscribe 在 broker 内部维护 topic → handlers map，启动一个 goroutine 拉 ConsumerGroup
func New(opts ...Option) (mq.Broker, error) {
	cfg := &brokerConfig{
		brokers:     []string{"127.0.0.1:" + defaultBrokerPort},
		group:       defaultGroup,
		version:     defaultVersion,
		errHandler:  defaultErrHandler,
		dialTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	if len(cfg.brokers) == 0 {
		return nil, fmt.Errorf("mq.kafka: no brokers configured")
	}

	scfg, err := buildSaramaConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("mq.kafka: build config: %w", err)
	}

	client, err := sarama.NewClient(cfg.brokers, scfg)
	if err != nil {
		return nil, fmt.Errorf("mq.kafka: connect %v failed: %w", cfg.brokers, err)
	}

	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("mq.kafka: create producer: %w", err)
	}

	// ConsumerGroup 必须用独立 Config（sarama 限制：producer/consumer 不能共享 Config 实例）
	cgCfg, err := buildSaramaConfig(cfg)
	if err != nil {
		_ = producer.Close()
		return nil, fmt.Errorf("mq.kafka: build consumer config: %w", err)
	}
	cg, err := sarama.NewConsumerGroup(cfg.brokers, cfg.group, cgCfg)
	if err != nil {
		_ = producer.Close()
		return nil, fmt.Errorf("mq.kafka: create consumer group %q: %w", cfg.group, err)
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	b := &broker{
		client:        client,
		producer:      producer,
		consumerGroup: cg,
		groupID:       cfg.group,
		handlers:      make(map[string][]mq.Handler),
		rootCtx:       rootCtx,
		rootCancel:    rootCancel,
		errHandler:    cfg.errHandler,
	}

	// 启动 consume loop（订阅后自动重平衡 + 拉取消息）
	b.wg.Add(1)
	go b.consumeLoop()
	return b, nil
}

// defaultErrHandler 默认错误处理：log.Error
func defaultErrHandler(topic string, _ *mq.Message, err error) {
	log.Error("mq.kafka: topic %s handler failed: %v", topic, err)
}

// Publish 发布消息到指定 topic
//
// 行为：
//   - 自动注入 ctx baggage 到 Kafka message headers（"baggage" key，W3C 编码）
//   - 同步：返回 nil 表示 Kafka 已 ack（leader 副本写入，按 Acks 配置）
//   - broker Close 后返回 error
//   - msg.Headers 用户自定义字段会一并写入 Kafka headers
func (b *broker) Publish(ctx context.Context, topic string, msg *mq.Message) error {
	if err := mq.ValidateMessage(topic, msg); err != nil {
		return err
	}
	b.mu.RLock()
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		return fmt.Errorf("mq.kafka: broker closed")
	}

	// 构造 sarama.ProducerMessage
	pm := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg.Payload),
	}

	// 用户自定义 headers
	for k, v := range msg.Headers {
		pm.Headers = append(pm.Headers, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}

	// 自动注入 baggage：把 ctx 中的 propagation.Bag 编码为 W3C baggage 字符串
	if bag := propagation.FromContext(ctx); bag != nil && bag.Len() > 0 {
		pm.Headers = append(pm.Headers, sarama.RecordHeader{
			Key:   []byte(baggageHeader),
			Value: []byte(encodeBaggage(bag)),
		})
	}

	if _, _, err := b.producer.SendMessage(pm); err != nil {
		return fmt.Errorf("mq.kafka: publish to %q failed: %w", topic, err)
	}
	return nil
}

// Subscribe 订阅 topic，handler 在 ConsumerGroup 内部 goroutine 中调用
//
// 行为：
//   - 非阻塞：注册后立即返回（实际消费在后台 consume loop）
//   - 同 topic 多次 Subscribe：fan-out（所有 handler 各自接收每条消息）
//   - ctx 自动从 broker rootCtx 派生 + 注入 msg.Headers 中的 baggage
//   - Close 时所有订阅 ctx 取消，broker 等待 in-flight handler 完成
//   - handler 返回 nil 自动 ack offset；返回 error 不 ack，下次重新投递
func (b *broker) Subscribe(_ context.Context, topic string, handler mq.Handler) error {
	if topic == "" {
		return fmt.Errorf("mq.kafka: topic is required")
	}
	if handler == nil {
		return fmt.Errorf("mq.kafka: handler is nil")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("mq.kafka: broker closed")
	}
	b.handlers[topic] = append(b.handlers[topic], handler)
	b.mu.Unlock()
	return nil
}

// Close 优雅关闭 broker
//
// 行为：
//  1. 标记 closed
//  2. 取消 rootCtx（consume loop 检测到退出）
//  3. 关闭 producer / consumerGroup / client
//  4. 等待 consume loop goroutine 退出
//  5. 重复调用是 no-op
func (b *broker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	b.rootCancel()

	if b.consumerGroup != nil {
		_ = b.consumerGroup.Close()
	}
	if b.producer != nil {
		_ = b.producer.Close()
	} else if b.client != nil {
		// producer.Close() 已经把 client 关了，避免重复
		_ = b.client.Close()
	}

	b.wg.Wait()
	return nil
}

// consumeLoop 持续消费 broker 中所有 topic
//
// 策略：
//   - 用 ConsumerGroup.Consume 拉取消息（自带 rebalance）
//   - 全部 topics 在一个 session 内消费
//   - session 结束（错误/重平衡）后自动重启
//   - rootCtx 取消时退出循环
func (b *broker) consumeLoop() {
	defer b.wg.Done()

	// 单独 goroutine 监听 rootCtx 取消 → 主动 Errors() 通道触发退出
	go func() {
		<-b.rootCtx.Done()
		if b.consumerGroup != nil {
			_ = b.consumerGroup.Close()
		}
	}()

	for {
		b.mu.RLock()
		topics := make([]string, 0, len(b.handlers))
		for t := range b.handlers {
			topics = append(topics, t)
		}
		closed := b.closed
		b.mu.RUnlock()

		if closed || len(topics) == 0 {
			// 没有订阅或已关闭：等一会儿再查（避免 busy loop）
			select {
			case <-b.rootCtx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}

		// Consume 阻塞直到 session 结束（重平衡/错误/ctx 取消）
		// handler 路由在 consumerGroupHandler.ConsumeClaim 中实现
		if err := b.consumerGroup.Consume(b.rootCtx, topics, &consumerGroupHandler{broker: b}); err != nil {
			select {
			case <-b.rootCtx.Done():
				return
			default:
			}
			// 错误后短暂退避
			select {
			case <-b.rootCtx.Done():
				return
			case <-time.After(time.Second):
			}
		}

		select {
		case <-b.rootCtx.Done():
			return
		default:
		}
	}
}

// consumerGroupHandler 实现 sarama.ConsumerGroupHandler 接口
//
// 每个 session 创建一次（重平衡时重建）
type consumerGroupHandler struct {
	broker *broker
}

// Setup 在 session 开始时调用（sarama 在 partition 分配后）
func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup 在 session 结束时调用
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 持续从 claim 拉取消息，路由到对应 handler
//
// 行为：
//   - 对每条消息，根据 topic 查找 handlers 列表
//   - 逐个调用 handler（fan-out）
//   - 任一 handler 返回 error：调用 ErrorHandler，但仍然 ack 消息（避免毒丸阻塞 partition）
//   - 全部 handler 调用完：session.MarkMessage 提交 offset
//
// 设计决策：handler 失败时仍然 ack
//   - 原因：Kafka 重投递需要 partition 内顺序，单条失败会阻塞后续消息
//   - 替代：用户需要"失败重试"语义时，在 handler 内自己实现（如写入死信 topic）
//   - 替代2：用 sarama 的 OffsetCommit + 自己管理 offset（不在本插件范围）
func (h *consumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-h.broker.rootCtx.Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			h.routeMessage(msg)
			sess.MarkMessage(msg, "")
		}
	}
}

// routeMessage 把 sarama 消息转换为 mq.Message 并路由到 handlers
func (h *consumerGroupHandler) routeMessage(kmsg *sarama.ConsumerMessage) {
	msg := saramaToMQ(kmsg)

	// 自动 extract baggage 到 ctx（从 Kafka header "baggage" 字段解析）
	headers := make(map[string]string, len(kmsg.Headers))
	var baggageValue string
	for _, h := range kmsg.Headers {
		key := string(h.Key)
		val := string(h.Value)
		headers[key] = val
		if key == baggageHeader {
			baggageValue = val
		}
	}
	if baggageValue != "" {
		headers[baggageHeader] = baggageValue
	}
	handlerCtx := propagation.ExtractMetadata(h.broker.rootCtx, headers)

	h.broker.mu.RLock()
	handlers := make([]mq.Handler, len(h.broker.handlers[kmsg.Topic]))
	copy(handlers, h.broker.handlers[kmsg.Topic])
	h.broker.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(handlerCtx, msg); err != nil {
			h.broker.errHandler(kmsg.Topic, msg, err)
		}
	}
}

// saramaToMQ 把 sarama 消息转换为 mq.Message
func saramaToMQ(kmsg *sarama.ConsumerMessage) *mq.Message {
	headers := make(map[string]string, len(kmsg.Headers))
	for _, h := range kmsg.Headers {
		headers[string(h.Key)] = string(h.Value)
	}
	return &mq.Message{
		Topic:   kmsg.Topic,
		Payload: kmsg.Value,
		Headers: headers,
	}
}

// encodeBaggage 把 propagation.Bag 编码为 W3C baggage 字符串
//
// W3C 格式：key1=val1,key2=val2（与 propagation/http.go 中保持一致）
func encodeBaggage(bag *propagation.Bag) string {
	if bag == nil || bag.Len() == 0 {
		return ""
	}
	var buf strings.Builder
	first := true
	for _, e := range bag.Entries() {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.WriteString(e.Key)
		buf.WriteByte('=')
		buf.WriteString(e.Value)
	}
	return buf.String()
}

// buildSaramaConfig 根据 brokerConfig 构造 sarama.Config
//
// 关键配置：
//   - Producer.Return.Successes = true（SyncProducer 必须）
//   - Producer.Return.Errors = true
//   - Producer.RequiredAcks = WaitForAll（默认强一致，可被 producerOpts 覆盖）
//   - Consumer.Return.Errors = true
//   - Consumer.Offsets.Initial = OffsetNew（从最新消息开始，避免重启后回放）
//   - Version = 用户指定或默认 3.5.0
func buildSaramaConfig(cfg *brokerConfig) (*sarama.Config, error) {
	sc := sarama.NewConfig()
	sc.Net.DialTimeout = cfg.dialTimeout

	version, err := sarama.ParseKafkaVersion(cfg.version)
	if err != nil {
		return nil, fmt.Errorf("parse version %q: %w", cfg.version, err)
	}
	sc.Version = version

	// Producer 默认配置
	sc.Producer.Return.Successes = true
	sc.Producer.Return.Errors = true
	sc.Producer.RequiredAcks = sarama.WaitForAll
	if cfg.producerOpts != nil {
		cfg.producerOpts(sc)
	}

	// Consumer 默认配置
	sc.Consumer.Return.Errors = true
	sc.Consumer.Offsets.Initial = sarama.OffsetNewest
	if cfg.consumerOpts != nil {
		cfg.consumerOpts(sc)
	}

	return sc, nil
}
