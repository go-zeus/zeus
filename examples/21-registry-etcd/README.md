# 21-registry-etcd · etcd 注册中心

演示用 `plugins/registry/etcd` 把服务实例注册到 etcd，进程退出时自动反注册（lease 到期 + 显式 Deregister 双保险）。本示例带 `docker-compose.yml` + `cmd/check` 校验工具。

## 演示场景

- components 自动装配 + etcd 注册中心
- `ServiceComponent.OnStart` 自动把 HTTP server 注册到 etcd（带 lease + KeepAlive）
- 退出时：正常 Ctrl+C → 显式 Deregister 立即删除；异常 `kill -9` → lease 到期（默认 30s）后删除

## 前置准备

```bash
# 本机 etcd（localhost:2379）
# 或远程 etcd，用环境变量指定：
#   ZEUS_ETCD_ENDPOINT=127.0.0.1:2379
```

或用自带 compose：`docker compose -f examples/21-registry-etcd/docker-compose.yml up -d etcd`

## 启动与测试

```bash
cd examples/21-registry-etcd
go run .
```

```bash
# 访问服务
curl http://localhost:18080/
curl http://localhost:18080/health

# 另开终端：查看 etcd 中已注册的实例（example 运行期间）
go run ./cmd/check
```

Ctrl+C 退出后，etcd 中对应 key 应在 Deregister 完成后立即删除。

## 核心代码

```go
import etcd "github.com/go-zeus/zeus/plugins/registry/etcd"

reg, err := etcd.New(
    etcd.WithEndpoints("127.0.0.1:2379"),
    etcd.WithTTL(30 * time.Second),
)
app := components.NewApp(
    components.NewRegistryComponent(reg),
    components.NewServerComponent(httpdriver.NewHTTP(httpdriver.Port(18080), httpdriver.Mux(mux))),
    components.NewServiceComponent(),
)
app.Run()
```

URL scheme 方式（注册 `etcd://` scheme）：

```go
import _ "github.com/go-zeus/zeus/plugins/registry/etcd"
reg, _ := app.NewFromURL("etcd://127.0.0.1:2379?ttl=30s")
```

## 衔接

- 内存注册中心（无需外部依赖） → [01-hello](../01-hello/)、[02-with-registry](../02-with-registry/)
- 完整集群部署 → [20-full-demo](../20-full-demo/)
