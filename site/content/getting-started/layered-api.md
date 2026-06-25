---
title: 4 层 API
weight: 30
---

Zeus 用 4 层 API 覆盖不同用户。**不允许越层泄漏概念**——L1 用户不应感知 Component / Container / Lifecycle 等内部接口名。

## 层级概览

| 层 | 用户 | 入口 | 暴露概念 | 目标 |
|---|---|---|---|---|
| **L1** | 学习者 / 单进程 demo | `zeus.Run(cfg, handler)` | 仅 `App` + `Config` | 5 行启动 |
| **L2** | 个人开发者 / 配置驱动 | `zeus.Run(cfgWithRegistry, handler)` | `App` + `Config` + `Registry`（URL） | 改配置即可 |
| **L3** | 小团队 / 代码定制 | `app.NewApp(opts ...AppOption)` | `Server` + `Logger` + `Registry` + ... | 类型装配 |
| **L4** | 定制需求 / 完全控制 | `components.NewApp(comps ...any)` | 全部组件接口 | 永久逃生通道 |

## L1：5 行启动

```go
app.Run(&app.Config{Port: 8080}, http.HandlerFunc(handler))
```

仅暴露 `App` 和 `Config` 两个概念。其余全部默认装配。

## L2：URL scheme 驱动

```go
import _ "github.com/go-zeus/zeus/plugins/registry/etcd"
import _ "github.com/go-zeus/zeus/plugins/cache/redis"

app.Run(&app.Config{
    Port:     8080,
    Registry: "etcd://localhost:2379",
    Cache:    "redis://localhost:6379",
}, handler)
```

L2 用 URL scheme（`etcd://` / `memory://` / `k8s://` / `redis://`）代替直接 import 实现包的细节构造。

## L3：类型装配

```go
a := app.NewApp(
    app.AddServer(http.NewHTTP(http.Port(8080), http.Mux(handler))),
    app.WithRegistry(memory.New()),
    app.WithMiddleware(recovery.New()),
    app.WithServiceCluster("canary"),
)
a.Run()
```

L3 是 L4 的"语法糖"，返回 `*components.App`，底层 100% 复用 Container/Lifecycle。

Option 清单：`AddServer` / `WithLogger` / `WithRegistry` / `WithMeter` / `WithTracer` / `WithMiddleware` / `WithServiceName` / `WithServiceCluster` / `WithServiceIP` / `WithStopTimeout` / `WithComponent` / `WithCacheURL` / `WithDatabaseURL` / `WithMQURL`。

## L4：声明式组件

```go
app := components.NewApp(
    components.NewLogComponent(slog.NewSlog()),
    components.NewRegistryComponent(memory.New()),
    components.NewServerComponent(http.NewHTTP(http.Mux(mux))),
    components.NewServiceComponent(),
)
app.Run()
```

L4 完整保留，作为永久逃生通道。组件声明依赖 → 拓扑排序 → 按序 Provide → OnStart → 逆序 OnStop。

## 混用（关键卖点）

L3 可与 L4 混用，参数末尾直接追加 L4 Component：

```go
a := app.NewApp(
    app.AddServer(http.NewHTTP()),
    app.WithMiddleware(recovery.New()),
    components.NewCacheComponent(myCache),       // L4 组件
    components.NewJobComponent(scheduler),       // L4 组件
)
```

## 何时升级层级

| 场景 | 推荐层级 |
|---|---|
| Demo / PoC | L1 |
| 单进程生产 | L1 或 L2 |
| 多实例 + 注册中心 | L2 |
| 自定义中间件链 | L3 |
| 多 Server（HTTP+gRPC 同进程） | L3 或 L4 |
| 完全控制组件生命周期 | L4 |

## 与 L1 的关键差异

- L1 自动包装 recovery/requestID/log/health/metrics 中间件；L3/L4 **不自动包装**
- 原因：L3/L4 用户已直接构造 Server，对中间件链有完全控制
- L3/L4 默认链需用户显式：`WithMiddleware(recovery.New())`

## 禁止规则

1. L1/L2 文档不能出现 `Component` / `Container` / `Lifecycle` / `Provide` / `Instance` 等内部接口名
2. L1 必须支持 0 配置启动
3. 新增功能先评估能否做默认，再考虑做成 Option
4. 类型推断优于显式选择
