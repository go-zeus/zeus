# 03-typed · L3 类型装配（app.NewApp + Option 模式）

用扁平化的 `WithXxx` Option 装配应用，类型安全且 IDE 可补全。L3 是 L4 的"语法糖"——底层 100% 复用 Container/Lifecycle，但不暴露 Component 概念。

## 演示场景

双 HTTP Server（业务 `:9080` + 管理 `:9081`）+ 自定义 Logger + recovery 中间件 + memory 注册中心。

## 启动与测试

```bash
cd examples/03-typed
go run .
```

```bash
# 业务 server
curl http://localhost:9080/ping       # pong
curl http://localhost:9080/panic      # 500（recovery 拦截 panic，不崩进程）

# 管理 server（独立端口）
curl http://localhost:9081/admin/health   # admin ok
```

## 核心代码

```go
a := app.NewApp(
    app.AddServer(http.NewHTTP(http.Port(9080), http.Mux(apiMux))),
    app.AddServer(http.NewHTTP(http.Port(9081), http.Mux(adminMux))),
    app.WithLogger(log.NewLogger(slog.NewSlog())),
    app.WithRegistry(memory.New()),
    app.WithMiddleware(recovery.New()),   // 中间件需用户显式装配（L3 不自动包装）
    app.WithServiceName("typed-demo"),
)
a.Run()
```

## 与 L4 的关键差异

| 维度 | L3（本示例） | L4（[05-autoapp](../05-autoapp/)） |
|---|---|---|
| 入口 | `app.NewApp(AddServer, WithXxx...)` | `components.NewApp(NewXxxComponent...)` |
| 暴露概念 | Server / Logger / Registry / Middleware | Component / Container / Lifecycle |
| 中间件 | 用户显式 `WithMiddleware` | 用户组装 MiddlewareComponent |
| 混用 | 参数末尾可直接追加 `components.NewXxxComponent(...)` | 完全控制 |

L3 → L4 渐进升级：在 `NewApp(...)` 末尾追加任意 L4 Component 即可，零适配成本。

## 衔接

- L2 配置驱动 → [04-config-driven](../04-config-driven/)
- L4 完全控制 → [05-autoapp](../05-autoapp/)、[06-autoapp-full](../06-autoapp-full/)
- 多 Server 单 App（L4）→ [07-autoapp-multi](../07-autoapp-multi/)
