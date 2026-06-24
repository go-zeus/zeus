// Package mq 提供消息队列（Message Queue）的统一抽象。
//
// 设计动机：
//   - 让"发布订阅"像 HTTP handler 一样声明式接入
//   - 主包零依赖，仅定义接口 + Message；内置 memory 实现作进程内事件总线
//   - 第三方实现（Kafka/NATS/Redis Streams）放 plugins/mq/<vendor>
//
// 设计权衡（参考 Dapr Building Block）：
//
//	抽象掉了：分区/offset/exchange/binding 等厂商专属语义
//	保留了：topic / payload / headers / ack 通过 Handler 返回 error 表达
//
// 适用场景：
//   - 业务事件分发（用户注册 → 发邮件 + 记日志 + 同步 CRM）
//   - 跨进程解耦（订单服务发布事件，库存/物流服务各自订阅）
//   - 进程内事件总线（memory 实现：测试 / 模块解耦）
//
// 不适用：
//   - 需要严格顺序 + 分区的流处理（用原生 Kafka SDK）
//   - 需要复杂路由（RabbitMQ exchange binding）
//   - 需要高吞吐 + 批量消费（直接用原生 SDK，控制 partition.assignment）
//
// 与 propagation 集成：
//
//	broker.Publish 时自动把 ctx 中的 baggage 注入 msg.Headers
//	subscriber.Handle 时自动从 msg.Headers 提取 baggage 到 ctx
//	→ 实现跨进程的 cluster / tenant.id 等 K-V 全链路传播
package mq

import (
	"context"
	"fmt"
)

// Message 消息结构
//
// 字段说明：
//   - Topic：消息目标 topic（发布时由 Publish 参数指定，订阅时回填）
//   - Payload：消息体（裸字节，序列化由调用方决定）
//   - Headers：元数据（含 propagation baggage 透传，以及用户自定义字段）
//
// 设计决策：
//   - Payload 用 []byte 而非泛型 T：避免接口分裂，序列化协议（JSON/Protobuf/Avro）由用户决定
//   - Headers 用 map[string]string 而非 map[string][]string：与 propagation 兼容，多数场景单值足够
type Message struct {
	Topic   string
	Payload []byte
	Headers map[string]string
}

// Handler 消息处理函数。
//
// 契约：
//   - 返回 nil 表示 ack（不同实现语义不同：memory 立即丢弃；Kafka commit offset）
//   - 返回 error 表示 nack（memory 走 ErrorHandler；Kafka 不 commit，下次重新投递）
//   - ctx 在 Subscriber.Stop 时被取消（用于优雅停止长时处理）
//   - ctx 自动注入 propagation baggage（从 msg.Headers 提取）
type Handler func(ctx context.Context, msg *Message) error

// ErrorHandler 消息处理失败时的钩子。
//
// 默认行为：log.Error。
// 用户可注入重试 / 死信队列 / 告警等逻辑。
type ErrorHandler func(topic string, msg *Message, err error)

// Publisher 消息发布者接口。
//
// 行为契约：
//   - 同步语义：返回 nil 表示 broker 已接收（不代表所有订阅者已处理）
//   - ctx 超时/取消：中断发布（已发到 broker 的不可撤回）
//   - 自动注入 baggage：实现者应调用 propagation.InjectMetadata(ctx, msg.Headers)
type Publisher interface {
	// Publish 发布一条消息到指定 topic
	Publish(ctx context.Context, topic string, msg *Message) error

	// Close 释放发布者资源（连接池等）
	Close() error
}

// Subscriber 消息订阅者接口。
//
// 行为契约：
//   - Subscribe 后立即返回（非阻塞），handler 在后台 goroutine 调用
//   - 同一 topic 多次 Subscribe：memory 实现 fan-out 给所有 handler；其他实现视厂商语义
//   - Stop 取消所有订阅 ctx，等待 handler 退出
type Subscriber interface {
	// Subscribe 订阅 topic，注册 handler
	Subscribe(ctx context.Context, topic string, handler Handler) error

	// Close 停止所有订阅并释放资源
	Close() error
}

// Broker 完整消息代理（同时实现 Publisher + Subscriber）。
//
// 典型场景：需要在统一对象上持有发布/订阅能力（共享连接池、配置等）。
// memory.New() 返回的实例同时满足 Broker 接口。
type Broker interface {
	Publisher
	Subscriber
}

// ValidateMessage 校验消息字段合法性（发布前由 Publisher 调用）
func ValidateMessage(topic string, msg *Message) error {
	if topic == "" {
		return fmt.Errorf("mq: topic is required")
	}
	if msg == nil {
		return fmt.Errorf("mq: message is nil")
	}
	return nil
}
