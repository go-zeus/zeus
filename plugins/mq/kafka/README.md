# Kafka 消息队列插件

`mq.Broker` 的 Apache Kafka 实现，基于 [IBM/sarama](https://github.com/IBM/sarama)（纯 Go，最成熟的 Kafka 库）。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/mq/kafka
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

### 1. 直接构造

```go
import (
    "context"
    kafkaplugin "github.com/go-zeus/zeus/plugins/mq/kafka"
    "github.com/go-zeus/zeus/propagation"
)

broker, err := kafkaplugin.New(
    kafkaplugin.WithBrokers("127.0.0.1:9092"),
    kafkaplugin.WithGroup("order-consumers"),
)
if err != nil {
    return err
}
defer broker.Close()

// 订阅（handler 返回 nil 自动 ack offset）
ctx := context.Background()
_ = broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
    tenantID, _ := propagation.Get(ctx, "tenant.id") // baggage 自动 extract
    return processOrder(tenantID, msg.Payload)
})

// 发布（ctx 中的 baggage 自动写入 Kafka header）
ctx = propagation.With(ctx, "tenant.id", "acme")
_ = broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("order-1")})
```

### 2. URL scheme 装配

```go
import _ "github.com/go-zeus/zeus/plugins/mq/kafka"

broker, err := mq.NewBrokerFromURL("kafka://h1:9092,h2:9092?group=order&version=3.7.0&timeout=5s")
```

URL 格式：
- `kafka://host:9092` — 单 broker
- `kafka://h1:9092,h2:9092,h3:9092` — 多 broker cluster
- `kafka://host:9092?group=xxx&version=3.7.0&timeout=5s` — query 参数

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithBrokers(addrs...)` | `...string` | `["127.0.0.1:9092"]` | Kafka broker 地址列表 |
| `WithGroup(g)` | `string` | `"zeus-default"` | consumer group ID（同 broker 实例绑一个 group） |
| `WithVersion(v)` | `string` | `"3.5.0"` | Kafka broker 版本字符串 |
| `WithErrorHandler(h)` | `mq.ErrorHandler` | `log.Error` | handler 失败钩子 |
| `WithProducerConfig(fn)` | `func(*sarama.Config)` | 强一致 | 覆盖 producer 配置（Acks/Compression/Idempotent 等） |
| `WithConsumerConfig(fn)` | `func(*sarama.Config)` | `Offsets.Initial=OffsetNew` | 覆盖 consumer 配置（Fetch.Min/Max 等） |
| `WithDialTimeout(d)` | `time.Duration` | `10s` | 连接超时 |

## 默认配置

**Producer**：
- `Return.Successes = true`（SyncProducer 必须）
- `Return.Errors = true`
- `RequiredAcks = WaitForAll`（强一致，等所有 ISR 副本）
- partition 路由：默认 round-robin（如需按 key 路由用 `WithProducerConfig` 自定义 `Partitioner`）

**Consumer**：
- `Return.Errors = true`
- `Offsets.Initial = OffsetNew`（从最新消息开始，避免重启后回放）
- group 内 partition 负载均衡

## ack 语义

| Handler 返回 | 行为 |
|--------------|------|
| `nil` | 自动提交 offset |
| `error` | 调用 ErrorHandler；**仍然 ack**（避免毒丸阻塞 partition） |

**设计决策**：handler 失败时仍然 ack，原因：
- Kafka 重投递需要 partition 内顺序，单条失败会阻塞后续消息
- 用户需要"失败重试"语义时，在 handler 内自己实现（如写入死信 topic）

## baggage 自动透传

| 位置 | 行为 |
|------|------|
| `Publish` 出口 | ctx baggage → Kafka header `"baggage"`（W3C 编码） |
| handler 入口 | Kafka header `"baggage"` → handler ctx |
| handler 内读取 | `propagation.Get(ctx, "tenant.id")` 直接拿到 |

## 依赖

- `github.com/IBM/sarama v1.43.3`（最新版，支持 Kafka 3.x）
- 间接：snappy / lz4 / zstd 压缩库（sarama 内置）

## 集成

- **trace/metrics**：本插件不直接接入（如需追踪请通过 baggage + `propagation.Get` 间接读 ctx）
- **示例**：参考 `examples/15-mq/`（用 memory 实现，业务代码无需改动即可切换到 kafka）
- **URL scheme**：`mq.NewBrokerFromURL("kafka://...")` 自动启用（需 `import _ "github.com/go-zeus/zeus/plugins/mq/kafka"`）

## 限制

- 单 broker 实例绑一个 consumer group（不同 group 各自构造 broker）
- 不抽象 partition / key 路由（用 `WithProducerConfig` 自定义）
- 不抽象事务 / ExactlyOnce 语义（用原生 sarama SDK）
- 不支持批量发布 / 消费（用原生 sarama SDK）
