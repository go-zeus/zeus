# 09-middleware · 中间件链独立用法

演示 `middleware.Chain` 通用接口的独立用法（不依赖 HTTP server）：构造链 + 显式调用 `chain.Handle`。适合测试中间件语义、学习拦截模式、自定义非 HTTP 协议的链。

## 演示场景

构造 `recovery + timeout` 链，对模拟请求执行，观察拦截顺序与超时/panic 行为。

## 启动与测试

```bash
cd examples/09-middleware
go run .
```

程序输出每次 Handle 的执行路径（recovery 外层 → timeout 内层 → handler），验证中间件顺序与拦截语义。

## 核心代码

```go
chain := middleware.NewChain(
    recovery.New(),
    timeout.New(2*time.Second),
)

// 显式执行链（HTTP server 用 httpdriver.ChainHandler 自动包装）
resp, err := chain.Handle(ctx, req, func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
    // 业务 handler
    return middlewareResponse{}, nil
})
```

## HTTP 服务的两种装配方式

```go
// 方式 1：L4 自动收集（简单场景，按字典序应用）
components.NewApp(
    components.NewMiddlewareComponent(recovery.New()),
    components.NewMiddlewareComponent(timeout.New(2*time.Second)),
    components.NewServerComponent(http.NewHTTP(http.Mux(handler))),
)

// 方式 2：手动 ChainHandler（严格顺序控制）
chain := middleware.NewChain(recovery.New(), timeout.New(2*time.Second))
srv := http.NewHTTP(http.Mux(httpdriver.ChainHandler(handler, chain)))
```

## 衔接

- 完整可观测中间件链 → [19-observability](../19-observability/)
- L4 自动装配 → [06-autoapp-full](../06-autoapp-full/)
