# zap 日志实现

`log.Writer` 的 [go.uber.org/zap](https://github.com/uber-go/zap) 实现。把 zeus 的高层日志 API 与 zap 的高性能结构化输出对接，支持生产级 JSON 输出、字段采样、级别过滤。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/log/zap
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
import (
    "github.com/go-zeus/zeus/log"
    zapimpl "github.com/go-zeus/zeus/plugins/log/zap"
)

w, err := zapimpl.New() // 默认 NewProduction：JSON + Info 级别 + sampling
if err != nil {
    panic(err)
}

logger := log.NewLogger(w)
defer logger.Close()

logger.Info("user login",
    log.String("user_id", "u-123"),
    log.Int("age", 28),
)
// {"level":"info","ts":...,"msg":"user login","user_id":"u-123","age":28}
```

复用已有 `*zap.Logger`（例如全局 logger、测试用 `NewExample`/`NewNop`）：

```go
custom := zapimpl.NewWith(myZapLogger) // 不接管生命周期，Close 只做 Sync
```

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithLevel(level)` | `zapcore.Level` | `InfoLevel` | 最小日志级别，低于此级别被丢弃 |

`New()` 内部使用 `zap.NewProduction()` 默认配置；如果需要自定义 encoder、输出目的地、采样策略等，请先用 `zap.New(...)` 构造自定义实例，再通过 `NewWith(z)` 注入。`LevelFatal` 会先 `Sync` 落盘再 `os.Exit(1)`，与 slog 实现行为对齐。

## 依赖

- `go.uber.org/zap`（核心库）
- `go.uber.org/zap/zapcore`（级别类型）

## 集成

- 与 `log.Logger` 配合：`log.NewLogger(zap.New())` 一行装配，业务侧继续用 `log.Info/Debug/Error` 高层 API
- 与 `LogComponent` 配合：`components.NewLogComponent(w)` 装入 zeus App，`OnStop` 自动 `Sync` 落盘
- 与 cluster 路由联动：`log.Logger.Log` 会自动注入 `cluster` 字段到 fields，本插件直接转 `zap.Any`，输出自然带 `cluster=canary` 标签（非 default 时）
- 与 propagation 联动：从 ctx 读取 baggage entries 自动转 Field
- 示例参考仓库 `examples/observability/`
