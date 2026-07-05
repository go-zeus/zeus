# 06-autoapp-full · L4 完整自动装配

在 [05-autoapp](../05-autoapp/) 最小版基础上增加自定义 server + recovery 中间件 + 优雅关闭超时，演示更接近生产的 L4 装配。

## 演示场景

声明式组装 6 个组件：Log + Registry + Middleware(recovery) + Server(自定义) + Service + StopTimeout。

## 启动与测试

```bash
cd examples/06-autoapp-full
go run .
```

```bash
curl http://localhost:8080/hello
# hello from zeus

# health（DefaultHandler 注册）
curl http://localhost:8080/health
```

## 核心代码

```go
app := components.NewApp(
    components.NewLogComponent(logslog.NewSlog()),
    components.NewRegistryComponent(memory.New()),
    components.NewMiddlewareComponent(recovery.New()),   // 注册后 ServerComponent 自动应用
    components.NewServerComponent(httpdriver.NewHTTP(httpdriver.Mux(mux))),
    components.NewServiceComponent(),
    components.WithStopTimeout(5*time.Second),            // 优雅关闭超时
)
app.Run()
```

关键点：`NewMiddlewareComponent` 注册后，`ServerComponent.OnStart` 自动收集所有 Interceptor 按字典序应用为中间件链——无需手动包装 handler。

## 衔接

- L4 最小版 → [05-autoapp](../05-autoapp/)
- L4 多 Server → [07-autoapp-multi](../07-autoapp-multi/)
- L3 类型装配 → [03-typed](../03-typed/)
