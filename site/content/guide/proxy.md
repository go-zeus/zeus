---
title: 反向代理
weight: 40
---

`proxy` 包提供多协议反向代理，统一 `http.Handler` 入口按协议自动嗅探分流。

```go
import (
    "net/http"
    "net/url"
    "github.com/go-zeus/zeus/proxy"
    "github.com/go-zeus/zeus/balancer/roundrobin"
)

// 静态模式
target, _ := url.Parse("http://127.0.0.1:9000")
p := proxy.New(proxy.WithSelector(proxy.NewStaticSelector(target)))
http.ListenAndServe(":8081", p)

// 动态模式（服务发现 + 集群路由）
p := proxy.New(proxy.WithSelector(
    proxy.NewDiscoverySelector("api-svc", dis, roundrobin.New()),
))
```

## 支持的协议

| 协议 | 实现 |
|---|---|
| **HTTP/HTTPS** | 基于 `httputil.ReverseProxy`，自动注入 `X-Forwarded-For`/`X-Real-IP`/`X-Request-ID` |
| **WebSocket** | Hijack + raw io.Copy 透传（nginx 风格，不解析 RFC6455 帧） |
| **SSE** | 禁用缓冲 + Flusher，串行 read-write-flush 保证事件顺序 |
| **gRPC** | 走独立 plugin 模块 `plugins/proxy/grpc`，独立监听端口（HTTP/2 多路复用） |

## Selector 接口

抽象后端选择：

| 实现 | 说明 |
|---|---|
| `NewStaticSelector(target *url.URL)` | 固定后端 |
| `NewDiscoverySelector(name, dis, lb)` | 动态服务发现 + 负载均衡 + 集群路由 |

扩展点：`WithDirector` / `WithResponseRewriter` / `WithErrorHandler` / `WithTransport`

## 内置负载均衡

| 包 | 算法 |
|---|---|
| `balancer/random` | 随机 |
| `balancer/roundrobin` | 轮询 |

实现 `Balancer` 接口，可在 `NewDiscoverySelector` 中替换。
