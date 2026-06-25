---
title: 快速开始
weight: 20
---

## 最小可运行示例

```go
package main

import (
    "net/http"

    "github.com/go-zeus/zeus/app"
)

func main() {
    app.Run(&app.Config{Port: 8080}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("hello from zeus"))
    }))
}

// curl http://localhost:8080
```

## 默认装配（零配置自动启用）

未配置时自动启用，用户零感知：

| 默认项 | 内置实现 |
|---|---|
| Server 协议 | 按 handler 类型推断（`http.Handler` → HTTP） |
| 注册中心 | `registry/memory`（L1） |
| 日志 | `log/slog` 输出到 stdout |
| 中间件 | recovery + requestID + 请求日志 |
| 健康检查 | `/health` `/health/ready` `/health/live` |
| Metrics | `/metrics`（noop meter 默认） |
| 信号处理 | SIGTERM/SIGINT/SIGQUIT → 优雅关闭（10s 超时） |
| 服务名 | `zeus-service`（可覆盖） |

**关键规则**：默认装配不允许失败。任何"必须配置才能跑"的字段都是设计缺陷。

## 添加业务路由

L1 入口接受任意 `http.Handler`，可以传入 `http.ServeMux`：

```go
func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", handleUsers)
    mux.HandleFunc("/api/orders", handleOrders)

    app.Run(&app.Config{Port: 8080, Name: "my-api"}, mux)
}
```

## 启用 Registry（L2 升级）

只需改 Config 中的 Registry URL 即可切换到分布式注册中心：

```go
import _ "github.com/go-zeus/zeus/plugins/registry/etcd"  // 注册 etcd:// scheme

app.Run(&app.Config{
    Name:     "my-service",
    Port:     8080,
    Cluster:  "canary",                    // 灰度集群
    Registry: "etcd://localhost:2379",
}, handler)
```

## 关闭与信号

应用自动监听以下信号：

- `SIGTERM` / `SIGINT` / `SIGQUIT` → 触发优雅关闭
- 默认 10s 关闭超时（可通过 `app.WithStopTimeout` 调整）

## 下一步

- [4 层 API 详解](layered-api) — 何时升级到 L2/L3/L4
- [配置指南](../guide/config) — Config 结构体字段说明
- [示例库](https://github.com/go-zeus/zeus/tree/main/examples) — 22 个完整示例
