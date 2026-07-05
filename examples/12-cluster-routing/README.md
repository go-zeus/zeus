# 12-cluster-routing · 集群路由端到端（X-Zeus-Cluster）

单进程演示 Zeus 集群路由核心能力：通过 `X-Zeus-Cluster` HTTP Header 端到端路由，"有标识走标识，无标识走 default"。这是 Zeus 区别于其他 Go 框架的关键能力（对齐 K8s/Istio/Envoy cluster 概念）。

## 演示场景

单进程内模拟完整调用链：

```
[Client] X-Zeus-Cluster: canary
   │
   ▼
[Gateway :8081] proxy 反向代理（按 cluster 选实例）
   │
   ▼
[srv-1] 入口注入 cluster 到 ctx → client 调下游时透传 Header
   │
   ▼
[srv-2 default | canary]   同名 Instance 按 Cluster 字段聚合为候选池
```

- `srv-2` 注册两个 cluster（default + canary），监听不同端口返回不同标识
- `gateway` 用 `proxy.NewDiscoverySelector` 按服务发现 + cluster 路由转发

## 启动与测试

```bash
cd examples/12-cluster-routing
go run .
```

```bash
# default 链路（无 Header → 走 default 实例）
curl http://localhost:8081/ping

# canary 链路（Header 路由到 canary 实例）
curl -H "X-Zeus-Cluster: canary" http://localhost:8081/ping
```

两次响应内容不同，证明流量按 cluster 隔离。

## 核心 API

```go
// 入口注入 cluster（server/http 默认自动做，本示例显式展示）
ctx := routing.WithCluster(r.Context(), routing.ClusterFromHTTPHeader(r.Header))

// 业务读取 cluster
cluster := routing.FromContext(ctx)

// client 调下游：自动透传 X-Zeus-Cluster Header（client.NewClient 内置）
cli := client.NewClient("srv-2", client.Discovery(dis), client.LoadBalance(roundrobin.New()))
```

| 常量 | 值 |
|---|---|
| `routing.HeaderCluster` | `X-Zeus-Cluster` |
| `routing.MetadataCluster` | `x-zeus-cluster`（gRPC metadata） |
| `routing.Default` | `default` |

## 与 20-full-demo 的区别

本示例是**单进程教学版**（default + canary 双 cluster，聚焦路由机制）；[20-full-demo](../20-full-demo/) 是**完整集群部署版**（gateway + api1 + srv1/2/3 四层链路 × 4 集群矩阵，含 K8s/Docker 编排）。

## 衔接

- W3C Baggage 全链路传播（cluster 之外的 K-V）→ [18-propagation](../18-propagation/)
- 完整集群部署演示 → [20-full-demo](../20-full-demo/)
