# gRPC Server

`server.Server` 接口的 gRPC 实现，基于官方 `google.golang.org/grpc`。默认从 incoming metadata `x-zeus-cluster` 自动提取集群标记并注入 context，配合 `propagation` 自动解析 W3C baggage，让 cluster 和任意 K-V 在业务 handler 中开箱可用。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/server/grpc
```

插件是独立 module，不会污染主仓依赖。

## 使用

L3 装配（推荐）：

```go
import (
    "github.com/go-zeus/zeus/app"
    "github.com/go-zeus/zeus/components"
    grpcserver "github.com/go-zeus/zeus/plugins/server/grpc"
    "google.golang.org/grpc"
)

func main() {
    a := app.NewApp(
        app.AddServer(grpcserver.NewGRPC(
            grpcserver.Port(9090),
            grpcserver.Register(func(s *grpc.Server) {
                pb.RegisterOrderServiceServer(s, &orderImpl{})
            }),
        )),
        app.WithServiceName("order-service"),
        app.WithRegistry(etcd.New()),
    )
    a.Run()
}
```

L1 入口（包装用户已构造好的 `*grpc.Server`）：

```go
import _ "github.com/go-zeus/zeus/plugins/server/grpc"

gs := grpc.NewServer()
pb.RegisterOrderServiceServer(gs, &orderImpl{})
app.Run(&app.Config{Port: 9090}, gs)
```

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Port(p)` | `int` | `9090` | 监听端口 |
| `Ip(ip)` | `string` | 空（监听所有接口） | 监听 IP |
| `Register(fn)` | `func(*grpc.Server)` | 无 | 注册业务 ServiceDesc 的回调，可多次调用按序执行 |
| `Interceptor(i ...)` | `[]grpc.UnaryServerInterceptor` | 空 | 追加自定义 unary 拦截器，cluster 拦截器始终最先执行 |
| `WithoutAutoClustering()` | — | 自动开启 | 关闭 `x-zeus-cluster` metadata → context 的自动注入 |

## 构造函数

| 函数 | 说明 |
|------|------|
| `NewGRPC(opts ...Option) server.Server` | 由插件内部创建 `*grpc.Server`，注入 cluster 拦截器后再执行 `Register` 回调 |
| `FromGRPC(srv *grpc.Server, opts ...Option) server.Server` | 包装用户已构造好的 `*grpc.Server`；autoClustering 默认关闭，不覆盖用户拦截器链 |

## 依赖

- `google.golang.org/grpc`，要求 Go ≥ 1.22
- 主仓 `server` / `routing` / `propagation` 包

## 集成

- 默认拦截器把 `x-zeus-cluster` 写入 `routing.WithCluster(ctx)`，业务侧 `routing.FromContext(ctx)` 直接读取
- 自动 `propagation.ExtractMetadataMulti` 解析 baggage，业务侧 `propagation.Get(ctx, "tenant.id")` 读取任意 K-V
- 与 `plugins/middleware/tracing` / `metrics` 配合，cluster 自动作为 span attribute 和 metrics label
- `ServiceComponent` 会为本 server 生成带 `Protocol="grpc"` 的 Instance 注册到 registry
- 参考示例：`examples/grpc-server/`
