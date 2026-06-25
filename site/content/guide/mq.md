---
title: 消息队列
weight: 90
---

`mq` 包提供发布/订阅（pub/sub）的统一抽象，参考 Dapr Building Block 设计。

| 概念 | 说明 |
|---|---|
| `Message` | 消息体（Topic + Payload + Headers） |
| `Handler` | 消息处理函数（返回 nil=ack，error=nack） |
| `Publisher` | 发布者接口（Publish + Close） |
| `Subscriber` | 订阅者接口（Subscribe + Close） |
| `Broker` | 完整代理（同时实现 Publisher + Subscriber） |
| `memory.Broker` | 内置实现：channel fan-out，无缓冲反压，自动 baggage 注入/提取 |
| `MQComponent` | components 适配器：声明式注册 + 自动启停 |

## 设计权衡

| 维度 | 选择 | 理由 |
|---|---|---|
| 抽象层级 | 只抽象 topic / payload / headers | 屏蔽 Kafka partition / RabbitMQ exchange 等厂商专属语义 |
| ack 语义 | Handler 返回 error = nack | 不同实现可映射到不同动作（memory 走 ErrorHandler，Kafka 不 commit offset） |
| 内置实现 | 进程内 channel + 无缓冲 | 零依赖、保证不丢消息，反压慢消费者（牺牲吞吐换可靠） |
| 持久化 | 不持久化 | 用作单进程事件总线 / 测试 mock；生产用 plugins |
| Baggage 传播 | Publish 自动注入 msg.Headers["baggage"]，handler 自动 extract | 全链路 tenant.id / cluster 等 K-V 透传 |
| 并发模型 | 每订阅者独立 goroutine | fan-out 隔离故障 |

## 使用方式

```go
import (
    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/mq"
    "github.com/go-zeus/zeus/mq/memory"
)

// 1. 直接使用 Broker（无 components）
broker := memory.New()
defer broker.Close()

_ = broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
    return nil
})
_ = broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("order-1")})

// 2. 自动装配
app := components.NewApp(
    components.NewMQComponent(memory.New()),
    components.NewMQSubscription("orders.created", handleOrder),
    components.NewMQSubscription("log.all", handleLog),
)
app.Run()
```

## Baggage 自动传播

| 位置 | 行为 |
|---|---|
| `Publish` 出口 | 自动 `InjectMetadata(ctx, msg.Headers)`：ctx baggage → `msg.Headers["baggage"]`（W3C 编码） |
| `handler` 入口 | 自动 `ExtractMetadata(ctx, msg.Headers)`：`msg.Headers["baggage"]` → handler ctx |
| `Handler` 内读取 | `propagation.Get(ctx, "tenant.id")` 直接拿到 |

完整示例参见 `examples/mq/`：3 个订阅者（不同 topic）+ baggage 全链路传播 + 优雅关闭。
