# 18-propagation · W3C Baggage 全链路传播

演示用户自定义 K-V（`tenant.id` / `feature.flag` / `region` 等）的全链路自动传播，扩展自单一 `X-Zeus-Cluster`。

## 演示场景

源头注入任意 K-V，框架自动透传：
- **server/http 入口**：自动 extract `Baggage` header → ctx
- **client 出口**：自动 inject ctx baggage → `Baggage` header
- **log**：自动从 ctx 读 baggage entries 写成 Field
- **tracing**：自动写 span attribute（每个 K-V 一个）

## 启动与测试

```bash
cd examples/18-propagation
go run .
```

```bash
# 带 baggage header 请求（模拟上游传播）
curl -H "Baggage: tenant.id=acme,feature.flag=beta" http://localhost:18091/api
```

服务端日志会自动带 `tenant.id=acme feature.flag=beta` 字段；下游 client 调用时 `Baggage` header 自动透传给后端。

## 核心代码

```go
// 业务注入 K-V（一次性）
ctx = propagation.With(ctx, "tenant.id", "acme")
ctx = propagation.With(ctx, "feature.flag", "beta")

// 业务读取
tenant, _ := propagation.Get(ctx, "tenant.id")
```

Zeus 抽象层（server/http、client、log、tracing）自动处理 inject/extract，绕过抽象（如直接用 `net/http.Client`）时需手动 `propagation.InjectHTTP(ctx, req.Header)`。

## 与 routing 的关系

`routing`（仅 `zeus.cluster` 单字段，HTTP Header `X-Zeus-Cluster`）是基于 propagation 的特化；本示例演示通用 K-V（W3C Baggage 标准）。

## 衔接

- 集群路由（cluster 单字段） → [12-cluster-routing](../12-cluster-routing/)
- 完整可观测（tracing 写 span attribute） → [19-observability](../19-observability/)
