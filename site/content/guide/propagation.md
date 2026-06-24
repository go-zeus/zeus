---
title: 上下文传播
weight: 25
---

`propagation` 包提供 W3C Baggage 兼容的跨进程 K-V 上下文传播，扩展自单一 `X-Zeus-Cluster` Header，支持用户自定义任意 K-V（如 `tenant.id` / `feature.flag` / `region`）的全链路透传。

## 与 routing 的关系

| 维度 | routing | propagation |
|------|---------|-------------|
| 范围 | 仅 `zeus.cluster` 单字段 | 任意 K-V |
| 协议 | `X-Zeus-Cluster` Header / gRPC metadata | `Baggage` Header（W3C 标准） |
| 实现 | 基于 propagation，同步到 Bag | W3C baggage 编解码 + Bag/Entry 数据结构 |

`routing.WithCluster` 同时写入 ctx 本地值 + propagation Bag，`routing.FromContext` 优先读 ctx 本地值，缺失时从 Bag 兜底。

## 自动传播矩阵（用户零感知）

| 位置 | 行为 |
|------|------|
| `server/http` 入口 | `clusterInjector` 自动 `ExtractHTTP` 注入 ctx |
| `plugins/server/grpc` 入口 | `clusterInterceptor` 自动 `ExtractMetadataMulti` |
| `client.Do` 出口 | 自动 `InjectHTTP` 写入 `Baggage` Header |
| `plugins/client/grpc` 出口 | `UnaryInterceptor` 自动 `InjectMetadataMulti` |
| `proxy` 反向代理 | HTTP Header 自然透传（`httputil.ReverseProxy` 默认行为） |
| `log` 包 | 自动从 ctx 读 baggage entries 写成 Field |
| `plugins/middleware/tracing` | 自动写 span attribute（每个 K-V 一个） |
| `plugins/middleware/metrics` | 默认不加 baggage label（避免基数爆炸），用户通过 `WithBaggageLabels` 显式声明 |

## 核心 API

```go
import "github.com/go-zeus/zeus/propagation"

// 业务代码注入 K-V（一次性）
ctx = propagation.With(ctx, "tenant.id", "acme")
ctx = propagation.With(ctx, "feature.flag", "beta")

// 业务代码读取
v, ok := propagation.Get(ctx, "tenant.id")

// 手动注入/提取（仅在不走 zeus client/server 时需要）
propagation.InjectHTTP(ctx, req.Header)
ctx = propagation.ExtractHTTP(ctx, r.Header)
```

## 不自动传播的场景

绕过 Zeus 抽象时需手动调用：

- 直接用 `net/http.Client.Do()` → 手动 `propagation.InjectHTTP(ctx, req.Header)`
- 直接用 `grpc.Dial()` → 手动 `propagation.InjectMetadataMulti(ctx, md)`
- 直接用 `kafka-go` / `sarama` 等 MQ 库 → 在消息 Header 中手动写入/读取

完整示例参见 `examples/propagation/`：演示 baggage 入站自动 extract + 业务注入 + log 自动带 Field。
