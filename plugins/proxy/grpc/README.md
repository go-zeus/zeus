# gRPC 反向代理

基于 grpc-go 的 `UnknownServiceHandler` 实现的 gRPC 透明反向代理，拦截所有 RPC 调用并双向透传 stream 到后端。由于 gRPC 走 HTTP/2 多路复用，本代理独立监听端口，不与主 `proxy` 包（HTTP/1.1 + WebSocket + SSE）混用同端口。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/proxy/grpc
```

插件是独立 module，不会污染主仓依赖。

## 使用

静态后端：

```go
import (
    "net/url"
    grpcproxy "github.com/go-zeus/zeus/plugins/proxy/grpc"
)

func main() {
    target, _ := url.Parse("127.0.0.1:9090")
    p, _ := grpcproxy.New(grpcproxy.WithTarget(target))
    _ = p.Listen(":8082")
    _ = p.Serve()
}
```

动态后端（结合服务发现 + 负载均衡）：

```go
import (
    "github.com/go-zeus/zeus/balancer/round_robin"
    "github.com/go-zeus/zeus/proxy"
    grpcproxy "github.com/go-zeus/zeus/plugins/proxy/grpc"
)

p, _ := grpcproxy.New(grpcproxy.WithSelector(
    proxy.NewDiscoverySelector("order-service", discovery, round_robin.New()),
))
_ = p.Listen(":8082")
_ = p.Serve()
```

`Selector` 接口签名：`Pick(method string) (*url.URL, error)`，可自行实现按 cluster / metadata 路由。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithTarget(u *url.URL)` | `*url.URL` | 必填（与 selector 二选一） | 静态后端地址（host:port） |
| `WithSelector(s selector)` | `selector` | 无 | 动态后端选择器，优先级高于 `WithTarget` |

`Proxy` 接口方法：

| 方法 | 说明 |
|------|------|
| `Listen(addr string) error` | 在指定 TCP 地址监听 |
| `Serve() error` | 启动 gRPC 服务，阻塞直到 `Stop()` |
| `Stop() error` | 优雅停止，调用 `GracefulStop` 等待活跃 RPC 结束 |

## 依赖

- `google.golang.org/grpc`，要求 Go ≥ 1.22
- 主仓 `proxy` 包（仅复用 `Selector` / `NewDiscoverySelector` 抽象，无循环依赖）

## 集成

- 客户端 incoming metadata 完整透传到后端（`X-Zeus-Cluster` / `baggage` 自然保留），cluster 路由链路不断
- 配合 `plugins/client/grpc` 的 `UnaryInterceptor`，后端调用下游服务时 cluster 继续透传
- 双向 stream 转发：任一方向 EOF / 错误会触发 cancel 唤醒对端，避免 goroutine 泄漏
- 每个请求独立 `grpc.NewClient` + `defer cc.Close()`，无连接池（适合流量较低的场景）
- 参考示例：`examples/proxy/`（HTTP 反向代理用法，gRPC 用法类似）
