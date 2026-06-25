---
title: 客户端
weight: 60
---

`client` 包提供带集群路由 + baggage 传播的 HTTP 客户端。

```go
import "github.com/go-zeus/zeus/client"

c := client.NewClient(
    client.WithDiscovery(dis),         // 启用服务发现
    client.WithBalancer(roundrobin.New()),
    client.WithTimeout(5*time.Second),
)
resp, err := c.Do(ctx, req)
```

## 自动行为

| 集成 | 行为 |
|---|---|
| 集群路由 | `resolveCluster` 读 ctx 的 cluster → 选 cluster → 注入 `X-Zeus-Cluster` Header |
| Baggage 传播 | 自动 `InjectHTTP(ctx, req.Header)` 写入 `Baggage` Header |
| Tracing | 自动创建 client span（如有 tracer） |
| Metrics | 自动记录 client request latency（如有 meter） |

## URL scheme 切换

| Scheme | 实现 |
|---|---|
| HTTP（默认） | `client.HTTPClient`（主仓） |
| gRPC | `plugins/client/grpc`（独立抽象） |

`type Client = HTTPClient` 别名保留向后兼容。

## gRPC 客户端

```go
import grpcclient "github.com/go-zeus/zeus/plugins/client/grpc"

c := grpcclient.NewClient(
    "my-service",
    grpcclient.WithDiscovery(dis),
    grpcclient.WithUnaryInterceptor(),
)
```

`UnaryInterceptor()` 自动从 ctx cluster 注入 outgoing metadata。
