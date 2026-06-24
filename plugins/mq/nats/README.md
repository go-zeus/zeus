# NATS 消息队列插件

`mq.Broker` 接口的 NATS 实现，基于 `nats-io/nats.go`。通过 NATS v2.2+ 原生 Headers 透传 propagation baggage，与 HTTP / gRPC 路径零差异。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/mq/nats
```

插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "context"

    "github.com/go-zeus/zeus/mq"
    "github.com/go-zeus/zeus/propagation"
    natsplugin "github.com/go-zeus/zeus/plugins/mq/nats"
)

func main() {
    broker, err := natsplugin.New(
        natsplugin.WithURL("nats://127.0.0.1:4222"),
    )
    if err != nil {
        panic(err)
    }
    defer broker.Close()

    ctx := context.Background()

    // 订阅（subject 支持通配符：orders.* / orders.>）
    _ = broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
        // ctx 已自动注入 baggage（如 tenant.id）
        tenant, _ := propagation.Get(ctx, "tenant.id")
        _ = processOrder(msg.Payload, tenant)
        return nil
    })

    // 发布（ctx 中的 baggage 自动注入 NATS Headers["Baggage"]）
    ctx = propagation.With(ctx, "tenant.id", "acme")
    _ = broker.Publish(ctx, "orders.created", &mq.Message{
        Payload: []byte(`{"id":1,"amount":100}`),
    })
}

func processOrder([]byte, string) error { return nil }
```

`Publish` 是同步调用（返回 nil 表示 NATS 服务器已接收，不保证所有订阅者已处理）。`Subscribe` 非阻塞，handler 在 NATS 内部 goroutine 中执行，同 subject 多次订阅自动 fan-out。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithURL(url)` | `string` | `nats://127.0.0.1:4222` | NATS 服务器地址，多服务器用逗号分隔 |
| `WithNATSOptions(opts...)` | `...nats.Option` | 无 | 透传原生 nats.Option，用于 `MaxReconnects` / `ReconnectWait` / TLS 等高级配置 |
| `WithErrorHandler(h)` | `mq.ErrorHandler` | `log.Error` | 订阅 handler 返回 error 时的回调，可对接告警 |
| `WithConnectTimeout(d)` | `time.Duration` | `5s` | 建立连接的超时 |

## 依赖

- `github.com/nats-io/nats.go`（Core NATS；JetStream 不在本插件抽象范围内）

## 集成

- baggage 自动透传：
  - `Publish` 出口：ctx baggage 自动编码为 W3C 字符串写入 NATS Header `"Baggage"`
  - 订阅 handler 入口：自动从 NATS Header 解码 baggage 注入 ctx
  - 业务 handler 内 `propagation.Get(ctx, "tenant.id")` 直接拿到
- 与 zeus client / server 的 HTTP / gRPC baggage 路径完全一致，跨协议互通无适配
- 优雅关闭：`Close()` 取消所有订阅 ctx → 调用 `nc.Drain()` 等所有 in-flight 消息处理完 → 关闭连接
- URL scheme：`import _ "plugins/mq/nats"` 后可用 `mq.NewBrokerFromURL("nats://127.0.0.1:4222?timeout=5s")`
- 不抽象 JetStream（持久化 / ack / 事务）与 QueueGroup 之外的复杂路由，需要时直接用原生 `nats.Conn`
- 完整端到端示例参考 `examples/mq/`
