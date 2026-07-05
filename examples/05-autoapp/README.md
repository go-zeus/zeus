# 05-autoapp · L4 自动装配（components.NewApp）

Zeus 的"完全控制"层：用 `components.NewApp` 声明式组装组件，框架负责拓扑排序与生命周期编排。L4 是永久逃生通道——L1/L2/L3 底层都走它。

## 演示场景

最小 L4 应用：4 个组件（Log + Registry + Server + Service），启动后默认监听 `:8080`。

## 启动与测试

```bash
cd examples/05-autoapp
go run .
```

```bash
curl http://localhost:8080           # 默认 handler 响应
curl http://localhost:8080/health    # 健康检查
```

## 核心代码

```go
app := components.NewApp(
    components.NewLogComponent(logslog.NewSlog()),
    components.NewRegistryComponent(memory.New()),
    components.NewServerComponent(),     // 默认 HTTP :8080
    components.NewServiceComponent(),    // 注册 Instance 到 Registry
)
app.Run()
```

组件在 `Provide` 阶段解析依赖（拓扑排序），`OnStart` 按序启动，`OnStop` 逆序关闭。

## 与 L3 的关键差异

L4 直接面对 Component / Container / Lifecycle 概念，控制最细但概念最重；L3 用 `app.NewApp(WithXxx)` 扁平化包装，隐藏这些概念。详见 [03-typed](../03-typed/)。

## 衔接

- L4 完整版（含自定义 server + recovery）→ [06-autoapp-full](../06-autoapp-full/)
- L4 多 Server → [07-autoapp-multi](../07-autoapp-multi/)
- L3 类型装配 → [03-typed](../03-typed/)
