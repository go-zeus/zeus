# 链路追踪中间件

`middleware.Interceptor` 的链路追踪实现，在请求入口自动创建 span，并把 cluster、baggage 透传到分布式追踪系统。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/middleware/tracing
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "github.com/go-zeus/zeus/middleware"
    "github.com/go-zeus/zeus/middleware/recovery"
    "github.com/go-zeus/zeus/plugins/middleware/tracing"
    "github.com/go-zeus/zeus/plugins/trace/otel"
)

func main() {
    tracer := otel.New(otel.WithServiceName("order-service"))
    // 链顺序：recovery → tracing（外→内）
    chain := middleware.NewChain(
        recovery.New(),
        tracing.New(tracer),
    )
    _ = chain
}
```

配合 `TraceComponent` 使用时，框架在优雅关闭阶段会调用 `tracer.Close()` 完成 OTel batch flush，避免丢失末尾 span。

## 自动注入的 span 属性

| 属性来源 | span attribute | 说明 |
|----------|----------------|------|
| `routing.FromContext(ctx)` | `zeus.cluster` | 仅在非 default 时写入，便于按集群过滤 |
| `propagation.FromContext(ctx)` 各 entry | 原 key（如 `tenant.id`） | 跳过 `zeus.cluster` 避免重复 |
| `req.Method() + " " + req.Path()` | span name | 例如 `GET /api/orders` |

## 选项

`New(tracer trace.Tracer)` 仅接受一个参数，行为由 tracer 自身决定：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `tracer` | `trace.Tracer` | 无（必填） | 传 `nil` 时中间件 no-op，不创建 span |

tracing 自身不再提供额外 Option；采样率、exporter、资源属性等通过 `trace.Tracer` 实现控制（参见 `plugins/trace/otel`）。

## 依赖

- 主仓 `github.com/go-zeus/zeus`（`middleware` / `trace` / `routing` / `propagation`）
- 无第三方依赖

## 集成

- 配合 `plugins/trace/otel` 提供 OTel 实现，也可对接 `trace/noop` 关闭追踪
- 与 `plugins/middleware/metrics` 配合形成可观测性三件套（log + trace + metrics）
- cluster 标识由 `server/http` 或 `plugins/server/grpc` 入口自动注入 ctx，tracing 直接消费
- baggage entries 由 `propagation` 自动从 HTTP Header / gRPC metadata extract，tracing 自动写入 span

完整端到端示例参考 `examples/observability/`。
