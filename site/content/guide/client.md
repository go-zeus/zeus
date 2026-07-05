---
title: 客户端
weight: 60
---

`client` 包提供带集群路由 + baggage 传播的 HTTP 客户端。第一参数 `name` 是服务名（用于服务发现查找实例）。

```go
import "github.com/go-zeus/zeus/client"

c := client.NewClient("my-service",
    client.Discovery(dis),               // 启用服务发现（registry.Discovery）
    client.LoadBalance(roundrobin.New()),// 负载均衡策略
    client.WithTimeout(5*time.Second),
)
// ctx 经由 req 携带；client.Do 自动透传 cluster + baggage
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://my-service/api", nil)
resp, err := c.Do(req)
```

## 自动行为

| 集成 | 行为 |
|---|---|
| 集群路由 | `resolveCluster` 读 ctx 的 cluster → 选 cluster 实例 → 注入 `X-Zeus-Cluster` Header |
| Baggage 传播 | 自动 `InjectHTTP(ctx, req.Header)` 写入 `Baggage` Header |
| Tracing | 自动创建 client span（如有 tracer） |
| Metrics | 自动记录 client request latency（如有 meter） |

## 其他 Option

| Option | 说明 |
|---|---|
| `WithHTTPClient(hc *http.Client)` | 注入底层 `*http.Client` |
| `WithTLS(cfg *tls.Config)` | 启用 TLS |
| `WithTransport(rt *http.Transport)` | 注入自定义 transport |
| `WithTimeout(d)` | 请求超时 |

`type Client = HTTPClient` 别名保留向后兼容。主包仅提供 HTTP 客户端——gRPC 等其他协议走 `plugins/client/<protocol>` 独立 module（HTTP/gRPC 请求模型本质不同，强行抽象会失去类型安全）。

## gRPC 客户端

`plugins/client/grpc` 不重新抽象客户端，仅提供 `UnaryInterceptor()` 注入到标准 `grpc.DialContext`：

```go
import (
    "google.golang.org/grpc"
    grpcclient "github.com/go-zeus/zeus/plugins/client/grpc"
)

conn, _ := grpc.DialContext(ctx, target,
    grpc.WithUnaryInterceptor(
        grpcclient.UnaryInterceptor(),  // 自动从 ctx 读 cluster → outgoing metadata["x-zeus-cluster"]
    ),
)
```
