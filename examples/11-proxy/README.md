# 11-proxy · 多协议反向代理网关

演示 `proxy` 包的多协议反向代理：监听 `:8081`，按协议自动嗅探分流（HTTP/WebSocket/SSE）。

## 演示场景

- **HTTP/HTTPS**：标准反向代理，自动注入 `X-Forwarded-For` / `X-Real-IP` / `X-Request-ID`
- **WebSocket**：Hijack + raw `io.Copy` 双向透传（nginx 风格，不解析 RFC6455 帧）
- **SSE**：禁用缓冲 + Flusher，串行 read-write-flush 保证事件顺序

gRPC 代理走独立 plugin `plugins/proxy/grpc`（HTTP/2 多路复用，需独立端口）。

## 启动与测试

```bash
cd examples/11-proxy
go run .
```

需要后端 `127.0.0.1:9000` 提供服务（或修改 `target`），然后：

```bash
curl http://localhost:8081/api                                             # HTTP 反代
curl -N -H "Accept: text/event-stream" http://localhost:8081/events        # SSE 反代
wscat -c ws://localhost:8081/ws                                            # WebSocket 反代
```

## 核心代码

```go
// 静态后端模式
target, _ := url.Parse("http://127.0.0.1:9000")
p := proxy.New(proxy.WithSelector(proxy.NewStaticSelector(target)))

// 动态模式（服务发现 + 负载均衡 + cluster 路由）
p := proxy.New(proxy.WithSelector(
    proxy.NewDiscoverySelector("api-svc", dis, roundrobin.New()),
))
http.ListenAndServe(":8081", p)
```

## 衔接

- 集群路由（gateway 模式） → [12-cluster-routing](../12-cluster-routing/)
- 完整网关部署 → [20-full-demo](../20-full-demo/)
