# etcd 注册中心

`registry.Registrar` / `registry.Discovery` / `registry.Watcher` 的 etcd 实现，基于官方 `go.etcd.io/etcd/client/v3`。注册单位是 `*types.Instance`，使用 lease + KeepAlive 自动续约，进程崩溃后 TTL 到期自动反注册。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/registry/etcd
```

插件是独立 module，不会污染主仓依赖。

## 使用

```go
import (
    "github.com/go-zeus/zeus/app"
    etcd "github.com/go-zeus/zeus/plugins/registry/etcd"
)

func main() {
    a := app.NewApp(
        app.AddServer(http.NewHTTP(http.Port(8080))),
        app.WithRegistry(etcd.New(
            etcd.WithEndpoints("10.0.0.1:2379", "10.0.0.2:2379"),
            etcd.WithTTL(30*time.Second),
        )),
        app.WithServiceName("order-service"),
    )
    a.Run()
}
```

也可通过 L1/L2 URL scheme 自动装配：在 `app.Config` 中写 `Registry: "etcd://10.0.0.1:2379"`，并 `import _ ".../plugins/registry/etcd"`。

## Key 结构

```
<Prefix>/<service>/<instance-id> = Instance JSON
/zeus/services/order-service/i-1   = {"id":"i-1","name":"order-service",...}
```

按 service 名前缀拉取全量实例（`GetService`），`Watch` 监听该前缀的变更事件并触发订阅者重新拉取。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithEndpoints(eps ...string)` | `[]string` | `["127.0.0.1:2379"]` | etcd 集群节点地址列表（host:port） |
| `WithTTL(d)` | `time.Duration` | `30s` | lease 有效期（必须 ≥ 5s）；进程退出后该时长内自动反注册 |
| `WithPrefix(p)` | `string` | `/zeus/services/` | key 前缀，用于多环境隔离（dev/staging/prod） |
| `WithCredentials(user, pass)` | `string, string` | 空 | 启用 etcd 用户名/密码鉴权 |
| `WithClient(cli)` | `*clientv3.Client` | 内部创建 | 注入外部 client 复用连接池；注入后 `Close` 不关闭它 |
| `WithDialTimeout(d)` | `time.Duration` | `30s` | 首次连接 etcd 的拨号超时（含 TCP+TLS+HTTP/2 协商） |

## 依赖

- `go.etcd.io/etcd/client/v3` v3.6.x，要求 Go ≥ 1.25
- 主仓 `registry` / `types` 包

## 集成

- 与 `components.NewRegistryComponent` 配合自动注册/反注册 Instance
- `Watch` 返回的信号触发 `proxy.NewDiscoverySelector` 等订阅者刷新本地实例列表
- Instance 上的 `Cluster` 字段参与 `X-Zeus-Cluster` 路由
- L2 入口自动识别 `etcd://` scheme，无需 import 实现包外的额外 API
- 参考示例：`examples/21-registry-etcd/`
