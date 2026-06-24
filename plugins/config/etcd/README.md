# etcd 配置中心

`config.Loader` 的 etcd 实现，把 etcd KV 当作配置中心使用：`Load` 拉取全量 KV，`Watch` 监听变更并返回最新快照。语义对齐 `config/file`，每次 `Next` 返回当前全量（非增量）。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/config/etcd
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
import (
    "github.com/go-zeus/zeus/config"
    "github.com/go-zeus/zeus/plugins/config/etcd"
)

loader := etcd.New(
    etcd.WithEndpoints("etcd-0.etcd:2379", "etcd-1.etcd:2379"),
    etcd.WithPrefix("/myapp/config/"),
    etcd.WithCredentials("root", "secret"),
)

cfg, err := config.NewConfig(loader)
if err != nil {
    panic(err)
}
defer cfg.Close()

dsn := cfg.Get("database/dsn") // key 为去掉 prefix 的相对路径
```

etcd 中的数据布局：

```
/ myapp / config /
  ├─ database / dsn      → "postgres://db:5432/prod"
  ├─ feature / flag      → "true"
  └─ log / level         → "info"
```

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithEndpoints(addrs ...string)` | `[]string` | `[127.0.0.1:2379]` | etcd 集群节点地址列表 |
| `WithPrefix(prefix)` | `string` | `/zeus/config/` | prefix 模式：加载前缀下全部 KV，key 去掉前缀返回 |
| `WithKey(key)` | `string` | 空 | key 模式：仅加载单个 key。与 `WithPrefix` 互斥，同时设置时本项优先 |
| `WithDialTimeout(d)` | `time.Duration` | `30s` | 首次连接 etcd 的拨号超时 |
| `WithCredentials(user, pass)` | `string, string` | 空 | 启用 etcd 用户名/密码鉴权 |
| `WithClient(cli)` | `*clientv3.Client` | 自建 | 注入外部 client，跳过本包拨号；`Stop` 不关闭它 |

拨号是惰性的：构造时不连接，首次 `Load`/`Watch` 才建立 gRPC 通道。`Watch` 内部把 etcd 的增量事件合并后触发一次 `Load`，所以业务侧拿到的永远是最新全量，避免增量应用顺序错误。

## 依赖

- `go.etcd.io/etcd/client/v3`（与 `plugins/registry/etcd` 共用同一版本）

## 集成

- 与 `config.Config` 配合：`config.NewConfig(etcd.New(...))` 自动完成首次加载 + 启动 watch goroutine + 解码到目标结构
- 与 `ConfigComponent` 配合：`components.NewConfigComponent(loader, &myCfg)` 装入 zeus App 即可实现声明式热更新
- 多实例共享同一 etcd：在 prefix 下用相对路径区分（如 `payment/config/`、`order/config/`），便于按服务隔离命名空间
- 示例参考 `examples/config/etcd/`（如有）或仓库 `examples/` 下相关 demo
