# 指标采集中间件

`middleware.Interceptor` 的请求计数实现，按 `cluster + method + status` 三维 label 统计 `zeus_requests_total`。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/middleware/metrics
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "github.com/go-zeus/zeus/middleware"
    "github.com/go-zeus/zeus/middleware/recovery"
    metricsmw "github.com/go-zeus/zeus/plugins/middleware/metrics"
    "github.com/go-zeus/zeus/plugins/metrics/prometheus"
)

func main() {
    meter := prometheus.New(prometheus.WithNamespace("zeus"))
    // 默认：仅 cluster/method/status 维度
    chain := middleware.NewChain(
        recovery.New(),
        metricsmw.New(meter),
    )
    _ = chain
}
```

需要按租户/地域细分时，显式声明白名单：

```go
chain := middleware.NewChain(
    recovery.New(),
    metricsmw.New(meter,
        metricsmw.WithBaggageLabels("tenant.id", "region"),
    ),
)
```

## 指标定义

| 指标名 | 类型 | Labels | 说明 |
|--------|------|--------|------|
| `zeus_requests_total` | counter | `cluster`, `method`, `status`，可选 baggage keys | 每次请求 +1 |

status 默认值：handler 正常返回且 `StatusCode()==0` 时记为 `200`；handler 返回 error 且无响应时记为 `500`。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithBaggageLabels(keys ...string)` | 字符串可变参数 | 空 | 声明哪些 baggage key 作为 metric label |

参数说明：

- `meter` 必填，传 `nil` 时中间件 no-op
- baggage 白名单空字符串会被忽略

### 基数风险提示

`WithBaggageLabels` 仅声明值域有限的 key（如 `tenant.id` / `region` / `env`）。**不要**把 `user.id` / `order.id` 等高基数值作为 label，否则 Prometheus label 组合爆炸会拖垮服务。

## 依赖

- 主仓 `github.com/go-zeus/zeus`（`middleware` / `metrics` / `routing` / `propagation`）
- 无第三方依赖

## 集成

- 配合 `plugins/metrics/prometheus` 提供 Prometheus 导出，也可对接 `metrics/noop` 关闭采集
- 与 `plugins/middleware/tracing` 配合形成可观测性三件套（log + trace + metrics）
- cluster 标识由入口中间件（`server/http` / `plugins/server/grpc`）自动注入 ctx
- baggage entries 由 `propagation` 跨服务透传，本中间件按白名单读取

完整端到端示例参考 `examples/observability/`。
