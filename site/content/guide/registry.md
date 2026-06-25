---
title: 注册中心
weight: 10
---

注册中心三层模型对齐 K8s Endpoints + Istio ServiceEntry：

| 概念 | 说明 |
|---|---|
| `Instance` | 实例（一个进程的一个端口），含 `Id/Name/Cluster/Protocol/Ip/Port/Metadata/Labels` |
| `Cluster` | 集群（同名+同 cluster 的实例集合，按 cluster 路由的候选池） |
| `ServiceEntry` | 逻辑服务（同名实例的集合） |

注册/反注册最小单位是 `*types.Instance`。多协议应用注册多条 Instance（每条带 `Protocol` 字段，如 `http`/`grpc`）。

## 内置实现

```go
import "github.com/go-zeus/zeus/registry/memory"

reg := memory.New()
```

`registry/memory` 是进程内实现，零依赖，主要用于单进程 demo 和单测 mock。

## plugins 实现

```go
import _ "github.com/go-zeus/zeus/plugins/registry/etcd"
```

通过 `import _` 副作用注册 `etcd://` scheme。

## URL scheme 切换

L2 用户通过 URL 字符串切换实现：

| Scheme | 实现 |
|---|---|
| `memory://` | `registry/memory` |
| `etcd://` | `plugins/registry/etcd` |
| `nacos://` | `plugins/registry/nacos`（待补） |

```go
app.Run(&app.Config{
    Registry: "etcd://127.0.0.1:2379?name=my-service",
}, handler)
```

## 核心 API

| 接口 | 职责 |
|---|---|
| `Registrar` | `Register(ctx, *Instance)` / `Deregister(ctx, *Instance)` |
| `Discovery` | `GetService(ctx, name)` / `Watch(ctx, name) <-chan Event` |
| `Watcher` | `Next() (Event, error)` / `Stop()` |

## 多协议注册

一个 App 持有多个 Server（HTTP + gRPC 同进程）时，每个 Server 对应一个 Instance：

```go
components.NewServiceComponent(
    components.WithServiceName("my-app"),
    components.WithServiceCluster("canary"),
)
// ServiceComponent 在 OnStart 时遍历所有 Instance 调用 Registrar.Register
```
