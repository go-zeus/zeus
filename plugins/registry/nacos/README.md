# Nacos 注册中心插件

`registry.Registrar` / `Discovery` / `Watcher` 的 [Nacos](https://nacos.io/) 实现，基于 [nacos-group/nacos-sdk-go/v2](https://github.com/nacos-group/nacos-sdk-go)。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/registry/nacos
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

### 1. 直接构造

```go
import (
    nacosplugin "github.com/go-zeus/zeus/plugins/registry/nacos"
    "github.com/go-zeus/zeus/types"
)

reg := nacosplugin.New(
    nacosplugin.WithServer("nacos.example.com", 8848),
    nacosplugin.WithNamespace("production"),
    nacosplugin.WithGroup("BIZ_GROUP"),
    nacosplugin.WithCredentials("nacos", "nacos_password"),
)
defer reg.Close()

// 注册实例
_ = reg.Register(ctx, &types.Instance{
    Name:     "my-app",
    IP:       "10.0.0.1",
    Port:     8080,
    Cluster:  "canary",
    Protocol: "http",
    Metadata: map[string]string{"version": "v1.0"},
})

// 服务发现
entry, _ := reg.GetService(ctx, "my-app")
for _, ins := range entry.Instances {
    fmt.Println(ins.IP, ins.Port, ins.Cluster)
}
```

### 2. URL scheme 装配（L1/L2）

```go
import _ "github.com/go-zeus/zeus/plugins/registry/nacos"

// 在 cfg.Registry 中填入 URL
cfg.Registry = "nacos://nacos.example.com:8848?namespace=production&group=BIZ_GROUP"
app.Run(ctx, cfg, handler)
```

URL 格式：
- `nacos://host:8848` — 单 server
- `nacos://h1:8848,h2:8848,h3:8848` — 多 server cluster
- `nacos://user:pass@host:8848` — 用户名密码鉴权
- `nacos://host:8848?namespace=xxx&group=YYY&ak=ACCESS&sk=SECRET` — query 参数

## 字段映射

| zeus.Instance 字段 | Nacos 字段 | 说明 |
|---|---|---|
| `ID` | InstanceId（Nacos 自动生成） | 业务侧用 IP:Port 做幂等键 |
| `Name` | ServiceName | 服务名 |
| `IP` / `Port` | Ip / Port | 实例地址 |
| `Cluster` | Metadata["zeus.cluster"] | **不**映射到 Nacos ClusterName（避免概念冲突） |
| `Protocol` | Metadata["zeus.protocol"] | 协议标识 |
| `Metadata` | Metadata | 用户自定义 K-V 一对一透传 |

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithServer(host, port)` | `string, int` | `127.0.0.1:8848` | Nacos server（可多次调用追加多 server） |
| `WithNamespace(ns)` | `string` | `""`（公共） | 命名空间 ID（环境隔离） |
| `WithGroup(g)` | `string` | `DEFAULT_GROUP` | 服务分组 |
| `WithCredentials(u, p)` | `string, string` | 空 | Nacos 用户名/密码鉴权（v2.x 默认开启） |
| `WithAccessKey(ak, sk)` | `string, string` | 空 | 阿里云 ACM 风格鉴权 |
| `WithNamingClient(c)` | `INamingClient` | 内部创建 | 注入外部 client（复用连接/单测 mock） |
| `WithClientParams(fn)` | `func(*ClientConfig)` | 默认 | 覆盖心跳间隔、超时、日志等高级参数 |

## 默认行为

- **临时实例（Ephemeral=true）**：SDK 心跳保活，进程崩溃后 30s 内 Nacos 自动下线
- **Healthy 实例过滤**：`GetService` 仅返回 `Healthy=true` 的实例
- **GroupName**：默认 `DEFAULT_GROUP`，可通过 `WithGroup` 或 URL query 覆盖
- **ClusterName**：固定 `DEFAULT`（Nacos 物理集群概念，与 zeus.Instance.Cluster 不同）

## 依赖

- `github.com/nacos-group/nacos-sdk-go/v2 v2.2.7`（最新稳定版）
- 间接：阿里云 SDK / gRPC / prometheus client（nacos-sdk 内部使用）

## 集成

- **URL scheme**：`nacos://` 自动启用（需 `import _ "github.com/go-zeus/zeus/plugins/registry/nacos"`）
- **与 etcd 形成"国际+国内"双栈注册中心覆盖**
- **集群路由**：注册到 Nacos 的实例自动支持 zeus 的 X-Zeus-Cluster 路由（Cluster 字段存在 Metadata 中）
- **示例**：参考 `examples/02-with-registry/`（用 memory 实现，业务代码无需改动即可切换到 nacos）

## 限制

- 一个 Nacos server cluster 内的所有实例共享同一个 `DEFAULT` ClusterName（zeus.Instance.Cluster 走 Metadata 通道）
- `Watch` 用 callback 模式 + channel 通知，订阅者收到通知后需自行 `GetService` 拉最新列表
- 不支持跨 namespace 的服务发现（不同 namespace 是隔离的，符合 Nacos 设计）
