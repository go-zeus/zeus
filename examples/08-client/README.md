# 08-client · HTTP 客户端 + 服务发现 + 集群路由

演示 `client.NewClient` 的核心能力：基于服务发现选择实例 + 负载均衡 + 自动透传 `X-Zeus-Cluster`。本示例**不监听端口**（纯客户端演示）。

## 演示场景

1. memory 注册中心注册一个实例（`demo` → `127.0.0.1:8080`）
2. 用 `client.NewClient` 创建带服务发现 + 负载均衡的 HTTP client
3. 发请求，client 自动解析实例 + 透传 cluster Header

## 启动与测试

先在某端口起一个回显 server（或用现有 server），再：

```bash
cd examples/08-client
go run .
```

程序会向注册的实例发请求并打印响应（或连接错误，取决于 8080 是否有 server）。

## 核心代码

```go
mem := memory.New()
mem.Register(ctx, &types.Instance{
    ID: "1", Name: "demo", Cluster: "default",
    IP: "127.0.0.1", Port: 8080,
})
dis := mem.(registry.Discovery)

cli := client.NewClient("demo",
    client.Discovery(dis),
    client.LoadBalance(random.New()),   // 或 roundrobin.New()
)

// client.Do 自动：解析实例 → 选 cluster → 注入 X-Zeus-Cluster Header
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://demo/api", nil)
resp, err := cli.Do(req)
```

## 衔接

- 集群路由端到端（client + gateway + srv）→ [12-cluster-routing](../12-cluster-routing/)
- gRPC 客户端 → 仓库 `plugins/client/grpc`
