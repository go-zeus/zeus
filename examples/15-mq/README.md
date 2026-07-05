# 15-mq · 消息队列（pub/sub）+ baggage 传播

演示 `mq` 包的声明式发布/订阅：内存 broker + 多订阅者 + baggage 全链路传播。使用 `mq/memory`，无需 Kafka/NATS。

## 演示场景

- 内存 broker + 3 个订阅者（`orders.created` / `log.all` / `audit.all`）
- components 自动装配：注册订阅 + 启动 broker + 优雅关闭
- baggage 传播：发布侧注入 `tenant.id=acme`，所有订阅侧自动读取

## 启动与测试

```bash
cd examples/15-mq
go run .
```

预期输出（每秒一组）：

```
[INFO] mq broker started with 3 subscription(s)
[INFO] publisher publishing order #1
[INFO] [orders.created] order id=1 tenant=acme
[INFO] [log.all] captured: order created id=1 tenant=acme
[INFO] [audit.all] audit trail: tenant=acme
```

Ctrl+C 优雅关闭（broker.Close 等待 in-flight handler 完成）。

## 核心代码

```go
// 发布侧：ctx baggage 自动注入到 msg.Headers["baggage"]
ctx = propagation.With(ctx, "tenant.id", "acme")
broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("order-1")})

// 订阅侧：handler ctx 自动 extract baggage
broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
    tenant, _ := propagation.Get(ctx, "tenant.id")   // "acme"
    return nil   // 返回 nil = ack
})
```

## 接入 Kafka / NATS

```go
import _ "github.com/go-zeus/zeus/plugins/mq/kafka"   // 或 nats
broker, _ := mq.NewBrokerFromURL("kafka://host:9092?group=g1")
```

## 衔接

- W3C Baggage 全链路 → [18-propagation](../18-propagation/)
- 周期任务 → [16-job](../16-job/)
