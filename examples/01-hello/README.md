# 01-hello · L1 入门（5 行 Hello World）

最小化入门：用 `app.Run` 零配置启动一个完整的微服务。演示 Zeus "5 行启动"承诺——内置日志、中间件、注册中心、健康检查、优雅关闭，用户代码只写业务 handler。

## 默认装配（用户零感知）

启动即获得：

- slog 日志（stdout，自动带 `request_id` / `ip` / `cluster` 字段）
- 中间件链：`requestid → accesslog → recovery`
- memory 注册中心 + 服务自动注册
- 健康检查端点（`/health`）
- SIGINT/SIGTERM 优雅关闭（10s 超时）

## 启动与测试

```bash
cd examples/01-hello
go run .
```

```bash
curl http://localhost:8080/hi
# hello from zeus L1

# 自定义 request id（验证中间件链透传）
curl -H "X-Request-ID: my-trace" http://localhost:8080/hi
# 日志中可见 request_id=my-trace

# 健康检查（DefaultHandler 自动注册）
curl http://localhost:8080/health

# 优雅关闭
kill -INT <pid>
```

## 核心代码

```go
app.Run(&app.Config{Port: 8080}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("hello from zeus L1"))
}))
```

仅暴露 `app.Run(cfg, handler)` + `app.Config{Port}`。无 Component / Container / Server 等内部概念。

## 衔接

- 想用 URL 切换注册中心 / cache / mq？→ [02-with-registry](../02-with-registry/)、[04-config-driven](../04-config-driven/)（L2）
- 想用类型安全的 Option 装配？→ [03-typed](../03-typed/)（L3）
- 想完全控制每个组件？→ [05-autoapp](../05-autoapp/)（L4）
