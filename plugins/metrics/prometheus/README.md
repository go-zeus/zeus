# Prometheus 指标实现

`metrics.Meter` 的 Prometheus 实现，基于 `prometheus/client_golang`，提供 Counter / Histogram / Gauge 三种指标类型与 `/metrics` HTTP 端点。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/metrics/prometheus
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "net/http"

    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/plugins/metrics/prometheus"
    "github.com/go-zeus/zeus/server/http"
)

func main() {
    meter := prometheus.New(prometheus.WithNamespace("zeus"))

    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        meter.Counter("api_calls_total", map[string]string{
            "method": r.Method,
        }).Inc()
        w.WriteHeader(http.StatusOK)
    })
    // 暴露 /metrics 给 Prometheus 抓取
    mux.Handle("/metrics", prometheus.HTTPHandler())

    app := components.NewApp(
        components.NewMetricsComponent(meter),
        components.NewServerComponent(http.NewHTTP(http.Mux(mux))),
    )
    app.Run()
}
```

## 设计要点

- 同 `(name, sortedLabelKeys)` 组合共享一个 `*CounterVec` / `*HistogramVec` / `*GaugeVec`，避免重复 register 导致 panic
- 用户 labels map 顺序无关，内部对 keys 排序后做 cache key
- 重复调用 `Counter/Histogram/Gauge(name, labels)` 返回同一个底层 vec 的句柄

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithRegisterer(r)` | `prometheus.Registerer` | `prometheus.DefaultRegisterer` | 用于测试隔离或多 registry |
| `WithNamespace(ns)` | `string` | 空 | 指标命名空间前缀，所有指标变为 `<ns>_<name>` |
| `WithSubsystem(sub)` | `string` | 空 | 子系统前缀，所有指标变为 `<ns>_<sub>_<name>` |
| `WithDefaultBuckets(buckets)` | `[]float64` | `prometheus.DefBuckets` | Histogram 默认 bucket 边界 |

`New(opts ...Option)` 返回 `metrics.Meter` 接口，`HTTPHandler()` 是包级函数返回 `/metrics` handler。

## 依赖

- `github.com/prometheus/client_golang`（prometheus 官方 SDK）
- 主仓 `github.com/go-zeus/zeus`（`metrics` 接口）

## 集成

- 配合 `components.NewMetricsComponent(meter)` 自动注入，业务侧通过 `meter.Counter/Histogram/Gauge` 直接使用
- 与 `plugins/middleware/metrics` 中间件配合自动采集 `zeus_requests_total`
- 与 `database/sql` / `cache/memory` 配合自动采集 `db_query_total` / `cache_op_total` 等
- 默认注册到 `prometheus.DefaultRegisterer`，Prometheus Agent 抓取 `/metrics` 即可

完整端到端示例参考 `examples/observability/` 与 `examples/metrics/`。
