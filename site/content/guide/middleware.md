---
title: 中间件
weight: 50
---

Zeus 中间件基于 `Interceptor` 接口和 `Chain` 类型组合。

## 内置中间件

| 包 | 职责 |
|---|---|
| `middleware/recovery` | panic 恢复（默认装配） |
| `middleware/requestid` | 请求 ID 注入（默认装配） |
| `middleware/accesslog` | 访问日志（默认装配） |
| `middleware/timeout` | 请求超时控制 |
| `middleware/clustering` | cluster 自动注入 |

## plugins 中间件

| 包 | 职责 |
|---|---|
| `plugins/middleware/tracing` | 自动 trace 埋点 + cluster attr |
| `plugins/middleware/metrics` | 自动 metrics 埋点 + cluster label |

## 链式组合

```go
import (
    "github.com/go-zeus/zeus/middleware"
    "github.com/go-zeus/zeus/middleware/recovery"
    metricsmw "github.com/go-zeus/zeus/plugins/middleware/metrics"
    tracingmw "github.com/go-zeus/zeus/plugins/middleware/tracing"
)

// 链顺序：外 → 内
chain := middleware.NewChain(
    recovery.New(),
    tracingmw.New(tracer),
    metricsmw.New(meter),
)
```

## L1 vs L3 默认链差异

| 层 | 默认中间件 | 说明 |
|---|---|---|
| **L1/L2** (`app.Run`) | requestid → accesslog → recovery（`/health` 等端点单独路由，非中间件） | 自动包装 |
| **L3** (`app.NewApp`) | 空（用户显式 `WithMiddleware`） | 用户完全控制 |

L3 不自动包装的原因：用户已直接构造 `http.NewHTTP()`，对 server 中间件链有完全控制。

## 自定义 Interceptor

```go
// Interceptor 是接口（非函数类型），需实现 Intercept + Name 两个方法
type Interceptor interface {
    Intercept(ctx context.Context, req Request, handler Handler) (Response, error)
    Name() string
}
// Request / Response / Handler 都是 middleware 包内定义的接口类型（非裸 *http.Request）
```

实现该签名即可作为中间件使用，无需任何额外抽象。
