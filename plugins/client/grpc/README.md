# gRPC Client 拦截器

为 `google.golang.org/grpc` 客户端提供 `UnaryClientInterceptor`，自动从 context 提取集群标记和 W3C baggage 注入到 outgoing metadata，与 zeus 服务端形成端到端的 `X-Zeus-Cluster` + baggage 传播链路。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/client/grpc
```

插件是独立 module，不会污染主仓依赖。

## 使用

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    grpcclient "github.com/go-zeus/zeus/plugins/client/grpc"
    "github.com/go-zeus/zeus/routing"
    "github.com/go-zeus/zeus/propagation"
)

func main() {
    cc, _ := grpc.NewClient("order-service:9090",
        grpc.WithUnaryInterceptor(grpcclient.UnaryInterceptor()),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    client := pb.NewOrderServiceClient(cc)

    // 调用时 ctx 中的 cluster / baggage 自动透传到下游
    ctx := propagation.With(ctx, "tenant.id", "acme")
    ctx = routing.WithCluster(ctx, "canary")
    _ = client.CreateOrder(ctx, &pb.CreateOrderRequest{})
}
```

## 行为细节

- ctx 中的 cluster 非 `default` 时使用 `Set` 写入 metadata（覆盖语义，避免重复条目）
- ctx 中 cluster 为 `default` 时不写入 metadata，避免污染下游日志
- baggage 仅在 ctx 中存在 K-V 时编码为 W3C 格式写入 `metadata["baggage"]`
- 用户已有的其他 metadata key 保留，仅更新 cluster / baggage

## 手动注入

不经过拦截器（如直接调用 `grpc.Invoke`）的场景，使用 `WithCluster`：

```go
ctx = grpcclient.WithCluster(ctx, "canary")
_ = cc.Invoke(ctx, "/pkg.svc/Method", req, resp)
```

## 依赖

- `google.golang.org/grpc`，要求 Go ≥ 1.22
- 主仓 `routing` / `propagation` 包

## 集成

- 与 `plugins/server/grpc` 的 `clusterInterceptor` 对称：客户端 outgoing 注入，服务端 incoming 提取
- ctx 中的 cluster 来源于 `routing.WithCluster`（业务代码）或上游 server 自动注入
- baggage 中的 K-V 自动透传给下游 log / trace / metrics：log 自动追加 Field，tracing 自动写 span attribute
- L1 入口 `zeus.Run` 内部的 HTTP client 同样具备此能力；本插件面向 gRPC 场景
- 参考示例：`examples/cluster_routing/`
