---
title: 可观测性
weight: 30
---

三件套联动模式：构造期注入 → 中间件链自动埋点 → 优雅关闭时 batch flush。

```go
import (
    "github.com/go-zeus/zeus/middleware"
    "github.com/go-zeus/zeus/middleware/recovery"
    metricsmw "github.com/go-zeus/zeus/plugins/middleware/metrics"
    tracingmw "github.com/go-zeus/zeus/plugins/middleware/tracing"
    "github.com/go-zeus/zeus/plugins/metrics/prometheus"
    "github.com/go-zeus/zeus/plugins/trace/otel"
)

meter := prometheus.New(prometheus.WithNamespace("zeus"))
tracer := otel.New(otel.WithServiceName("my-app"))

// 链顺序：recovery → tracing → metrics（外→内）
chain := middleware.NewChain(recovery.New(), tracingmw.New(tracer), metricsmw.New(meter))

// /metrics 端点：mux.Handle("/metrics", prometheus.HTTPHandler())
// 优雅关闭时 TraceComponent.OnStop 调用 tracer.Close()，OTel batch flush 完成
```

## Metrics

| 接口 | 职责 |
|---|---|
| `Meter` | 工厂接口，创建 Counter/Histogram/Gauge |
| `Counter` | 单调递增计数器 |
| `Histogram` | 分布统计（请求耗时等） |
| `Gauge` | 瞬时值（连接数等） |

内置：`metrics/noop`（默认装配时使用）
插件：`plugins/metrics/prometheus`

## Trace

| 接口 | 职责 |
|---|---|
| `Tracer` | 创建 Span |
| `Span` | 单次操作单元，支持 attrs/end/recordError |

内置：`trace/noop`
插件：`plugins/trace/otel`

## Log

| 类型 | 职责 |
|---|---|
| `Writer` | 输出目标（io.Writer 风格） |
| `Logger` | 用户 API（With/Close） |
| `Field` | 结构化字段 |

内置：`log/slog`（自动注入 cluster Field）
插件：`plugins/log/zap`

## 自动行为矩阵

| 组件 | cluster 注入 | baggage 注入 |
|---|---|---|
| `tracing` 中间件 | span attr `zeus.cluster` | 每个 K-V 一个 attr |
| `metrics` 中间件 | label `cluster` | 默认不加（避免基数爆炸，`WithBaggageLabels` 显式声明） |
| `log` 包 | Field `cluster`（仅非 default） | 每个 baggage entry 一个 Field |

完整端到端示例参见 `examples/observability/`。
