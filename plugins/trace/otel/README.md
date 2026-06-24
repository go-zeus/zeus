# OpenTelemetry 链路追踪实现

`trace.Tracer` 的 OpenTelemetry 实现，桥接到 `go.opentelemetry.io/otel`，支持 OTLP / Jaeger / stdout 等 exporter，并可让第三方 OTel-instrumented 库自动接入。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/trace/otel
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "context"

    "github.com/go-zeus/zeus/components"
    otelplug "github.com/go-zeus/zeus/plugins/trace/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
)

func main() {
    exporter, err := otlptracegrpc.New(context.Background(),
        otlptracegrpc.WithEndpoint("otel-collector:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        panic(err)
    }

    tracer := otelplug.New(
        otelplug.WithServiceName("order-service"),
        otelplug.WithServiceVersion("v1.2.0"),
        otelplug.WithExporter(exporter),
        otelplug.WithResourceAttrs(map[string]string{
            "deployment.environment": "production",
        }),
    )

    app := components.NewApp(
        components.NewTraceComponent(tracer),
        // server / registry / ... 其他组件
    )
    app.Run()
}
```

`TraceComponent` 在优雅关闭阶段调用 `tracer.Close()`，触发 OTel batch flush，避免丢失末尾 span。

## 设计要点

- 桥接 zeus `trace.Tracer` 到 otel `trace.Tracer`，保留 ctx 注入语义，下游 otel-aware 库可继承父链
- 资源属性遵循 OTel semconv：`service.name` / `service.version` + 自定义 attrs
- 初始化惰性：首次 `StartSpan` 时才建立 provider 与 exporter 连接
- 初始化失败时返回 noop span，业务流程不受影响
- 默认注册为全局 `TracerProvider`，第三方 OTel 库自动接入

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithServiceName(name)` | `string` | `"zeus"` | 设置 `service.name` 资源属性 |
| `WithServiceVersion(v)` | `string` | 空 | 设置 `service.version` 资源属性 |
| `WithResourceAttrs(attrs)` | `map[string]string` | 空 | 追加资源属性（如 `host.name` / `deployment.environment`） |
| `WithSampler(s)` | `sdktrace.Sampler` | `AlwaysSample()` | 自定义采样器，生产建议 `ParentBased(TraceIDRatioBased(0.1))` |
| `WithExporter(e)` | `sdktrace.SpanExporter` | stdout（写 stderr） | 自定义 exporter（OTLP / Jaeger / stdout） |
| `WithStdoutWriter(w)` | `io.Writer` | `os.Stderr` | 仅未指定 `WithExporter` 时生效，控制 stdout 输出目标 |

`New(opts ...Option)` 返回 `trace.Tracer` 接口。

## 依赖

- `go.opentelemetry.io/otel`（otel core / sdk / stdouttrace / semconv）
- 主仓 `github.com/go-zeus/zeus`（`trace` 接口）

## 集成

- 配合 `components.NewTraceComponent(tracer)` 自动注入 + 优雅关闭 flush
- 与 `plugins/middleware/tracing` 中间件配合，每个请求自动起 span 并写入 `zeus.cluster` attribute
- 与 `database/sql` / `cache/memory` 配合自动产生 `db.query` / `cache.get` 等子 span
- 与 `plugins/middleware/metrics` 配合形成可观测性三件套（log + trace + metrics）

完整端到端示例参考 `examples/observability/` 与 `examples/trace/`。
