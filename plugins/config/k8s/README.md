# Kubernetes ConfigMap 配置加载

`config.Loader` 的 Kubernetes ConfigMap 实现。把单个 ConfigMap 的 `data` / `binaryData` 字段映射为 zeus 的 `KeyValue`，并通过 client-go Watch API 监听变更。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/config/k8s
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

声明 ConfigMap：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-app-config
  namespace: default
data:
  database/dsn: "postgres://localhost:5432/prod"
  feature.flag: "true"
  log/level: "info"
```

在 pod 内加载（自动用 ServiceAccount 做 in-cluster 鉴权）：

```go
import (
    "github.com/go-zeus/zeus/config"
    "github.com/go-zeus/zeus/plugins/config/k8s"
)

loader, err := k8s.New(
    k8s.WithName("my-app-config"),
    k8s.WithNamespace("default"),
)
if err != nil {
    panic(err)
}

cfg, _ := config.NewConfig(loader)
defer cfg.Close()

dsn := cfg.Get("database/dsn") // data["database/dsn"]
```

本地开发连远程集群时，通过 `WithKubeconfig` 或环境变量 `KUBECONFIG` 指向 kubeconfig 文件即可。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithName(name)` | `string` | 必填 | 监听的 ConfigMap 名称 |
| `WithNamespace(ns)` | `string` | `default` | ConfigMap 所在命名空间 |
| `WithKubeconfig(path)` | `string` | 空 | kubeconfig 文件路径（开发模式）。空时按 `KUBECONFIG` env / `~/.kube/config` 解析 |
| `WithClient(c)` | `kubernetes.Interface` | 自建 | 注入外部 clientset，跳过本包拨号；`Stop` 不关闭它 |

鉴权优先级：注入 client > in-cluster（pod 内 ServiceAccount） > kubeconfig 文件 > `~/.kube/config`。Watch 使用 client-go 长连接，server 端超时关闭后会自动重建，业务侧无感。

## 依赖

- `k8s.io/client-go`（kubernetes 客户端）
- `k8s.io/api` / `k8s.io/apimachinery`（核心类型与错误）

## 集成

- 与 `config.Config` 配合：`config.NewConfig(loader)` 自动完成首次加载 + Watch goroutine + 解码
- 与 `ConfigComponent` 配合：装入 zeus App 即声明式热更新，无需手动管理生命周期
- ConfigMap 不存在时 `Load` 返回空 KV（与 `config/file` 行为一致），便于灰度上线
- 支持 `data` 与 `binaryData` 两种字段，二进制数据（如证书、密钥）原样返回 `[]byte`
- 示例参考仓库 `examples/` 下相关 demo
