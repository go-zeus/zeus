# 10-config · 配置加载（config/file）

演示 `config` 包的配置加载能力：用 `file` loader 从文件读取配置，业务解码后使用。本示例是非 server 的独立脚本（跑完即退出）。

## 演示场景

1. 写一份示例 JSON 配置文件
2. 用 `config.NewConfig(file.NewFileWithPath(...))` 加载
3. `cfg.Get(key)` 读取原始值，或 `json.Unmarshal` 解码到 struct

## 启动与测试

```bash
cd examples/10-config
go run .
```

预期输出：

```
raw app.json: {"name":"zeus-demo","version":"1.0.0"}
decoded: name=zeus-demo version=1.0.0
```

## 核心代码

```go
cfg, err := config.NewConfig(file.NewFileWithPath("app.json"))
if err != nil { log.Fatal(err) }

raw := cfg.Get("app.json")              // 原始 []byte
var appCfg AppConfig
json.Unmarshal(raw, &appCfg)            // 标准解码
```

## 进阶

- 动态配置（watch）：`cfg.Watch(key, callback)` 监听变更
- 其他 loader：`plugins/config/etcd`（KV 配置树）、`plugins/config/k8s`（ConfigMap）

## 衔接

- L2 配置驱动（URL scheme）→ [04-config-driven](../04-config-driven/)
- 完整 server + 配置 → [06-autoapp-full](../06-autoapp-full/)
