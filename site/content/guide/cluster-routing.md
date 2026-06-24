---
title: 集群路由
weight: 20
---

并行开发场景下，多个项目共享同一套服务，每个项目对应一组灰度实例（cluster）。流量通过 `X-Zeus-Cluster` Header 端到端路由：**"有标识走标识，无标识走 default"**。

术语统一为 `cluster`，与 K8s/Istio/Envoy/gRPC xDS 对齐。

## 传播链路

```
[Client] X-Zeus-Cluster: canary
   │
   ▼ HTTP Header / gRPC metadata["x-zeus-cluster"]
[Gateway (proxy)] NewDiscoverySelector 从 Header 读 cluster → 选 cluster → 转发
   │
   ▼ Header 透传
[HTTP Server (srv-1)] 入口 clusterInjector 自动注入 ctx
   │   ├─ tracing: span.Attrs["zeus.cluster"]=cluster
   │   ├─ metrics: labels["cluster"]=cluster
   │   └─ log: 自动 Field{cluster}
   ▼
[业务 handler] ctx 已含 cluster
   │
   ▼ client.Do(): resolveCluster 读 ctx → 选 cluster → 注入 Header
[HTTP Server (srv-2)] 同 srv-1
```

## 核心 API

```go
import "github.com/go-zeus/zeus/routing"

// HTTP 入口注入
ctx := routing.WithCluster(r.Context(), routing.ClusterFromHTTPHeader(r.Header))

// 业务读取
c := routing.FromContext(ctx)

// 常量
routing.HeaderCluster   // "X-Zeus-Cluster"
routing.MetadataCluster // "x-zeus-cluster"（gRPC metadata）
routing.Default         // "default"
```

## 默认行为

| 组件 | 行为 |
|---|---|
| `server/http` | 默认自动注入 cluster（`WithoutAutoClustering()` 关闭） |
| `plugins/server/grpc` | 默认 UnaryServerInterceptor 从 metadata 提取 cluster 注入 ctx |
| `plugins/client/grpc` | `UnaryInterceptor()` 从 ctx cluster 注入 outgoing metadata |
| `log` 包 | 自动 prepend `Field{cluster}`（仅非 default 时） |
| `plugins/middleware/tracing` | 自动写入 span attribute `zeus.cluster` |
| `plugins/middleware/metrics` | 自动打 label `cluster` |

## 治理模块按 cluster 维度

```go
import clusterlimit "github.com/go-zeus/zeus/ratelimit/cluster"
import clusterbreak "github.com/go-zeus/zeus/circuitbreaker/cluster"
import clusterretry "github.com/go-zeus/zeus/retry/cluster"

// 每个 cluster key 独立桶/熔断器/重试策略
limiter := clusterlimit.New(func() ratelimit.Limiter { return token.New(100, 10) })
ok := limiter.Allow(ctx) // 从 ctx 提取 cluster 作为 key

cb := clusterbreak.New(func() circuitbreaker.Breaker { return counter.New(100, 0.5) })
err := cb.Execute(ctx, func() error { ... })

cr := clusterretry.New(func() retry.Retrier { return exponential.New(3, 100*time.Millisecond) })
r := cr.NewRetriever(ctx)
```

## 完整示例

参见 `examples/cluster_routing/`：单进程演示 gateway → srv1 → srv2 多 cluster 路由 + cluster 全链路传播。

```bash
curl http://localhost:8081/ping                                # default 链路
curl -H "X-Zeus-Cluster: canary" http://localhost:8081/ping    # canary 链路
```
