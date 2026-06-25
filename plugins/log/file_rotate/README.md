# 日志文件轮转插件

`log.Writer` 的文件轮转实现，基于 [gopkg.in/natefinch/lumberjack.v2](https://github.com/natefinch/lumberjack)，支持按大小切分、备份压缩、保留数量管理。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/log/file_rotate
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

### 1. 基础用法（JSON 格式 + 默认轮转）

```go
import (
    "github.com/go-zeus/zeus/log"
    "github.com/go-zeus/zeus/plugins/log/file_rotate"
)

w, err := file_rotate.New("/var/log/zeus/app.log")
if err != nil {
    return err
}

logger := log.NewLogger(w)
defer logger.Close()

logger.Info("user logged in", "user_id", 123)
// 输出：{"time":"2026-06-23T10:00:00Z","level":"INFO","msg":"user logged in","user_id":123}
```

### 2. 自定义轮转策略

```go
w, err := file_rotate.New("/var/log/zeus/app.log",
    file_rotate.WithMaxSize(100),      // 100 MB 切分
    file_rotate.WithMaxBackups(7),     // 保留 7 份
    file_rotate.WithMaxAge(30),        // 保留 30 天
    file_rotate.WithCompress(true),    // gzip 压缩
    file_rotate.WithLocalTime(true),   // 备份文件名用本地时间
)
```

### 3. TEXT 格式（调试场景）

```go
w, err := file_rotate.New("/var/log/zeus/app.log",
    file_rotate.WithFormat(file_rotate.FormatText),
)
// 输出：2026-06-23T10:00:00Z\tINFO\thello\tuser_id=123
```

### 4. 与 app.NewApp 集成

```go
import (
    "github.com/go-zeus/zeus/app"
    "github.com/go-zeus/zeus/plugins/log/file_rotate"
)

w, _ := file_rotate.New("/var/log/zeus/app.log",
    file_rotate.WithMaxSize(100),
    file_rotate.WithCompress(true),
)

a := app.NewApp(
    app.AddServer(http.NewHTTP()),
    app.WithLogger(log.NewLogger(w)),
)
```

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithMaxSize(mb)` | `int` | 100 | 单文件最大 MB（达到后切分） |
| `WithMaxBackups(n)` | `int` | 7 | 保留旧文件数量 |
| `WithMaxAge(days)` | `int` | 30 | 旧文件保留天数（0 = 不限） |
| `WithCompress(b)` | `bool` | false | gzip 压缩旧文件 |
| `WithLocalTime(b)` | `bool` | false | 备份文件名用本地时间（默认 UTC） |
| `WithFormat(f)` | `Format` | FormatJSON | 输出格式（JSON/Text） |
| `WithMinLevel(l)` | `log.Level` | LevelDebug | 最小日志级别（低于的丢弃） |

## 输出格式

### JSON（默认）

```json
{"time":"2026-06-23T10:00:00.123456789Z","level":"INFO","msg":"user logged in","user_id":123,"cluster":"canary"}
```

字段顺序：`time` → `level` → `msg` → 用户 fields
- `time`：RFC3339Nano UTC
- `level`：DEBUG/INFO/WARN/ERROR/FATAL（与 slog 对齐）
- 用户 fields：用户传入的 `Field{Key, Value}` 一对一透传

### TEXT

```
2026-06-23T10:00:00Z\tINFO\thello\tuser_id=123\tcluster=canary
```

字段间用 `\t` 分隔（便于 grep/awk）。

## 切分行为

满足任一条件触发切分：
- 文件大小达到 `MaxSize` MB
- 当前文件不是当天创建（`MaxAge` 控制保留天数，不触发按天切分）

切分后：
1. 当前文件重命名为 `app-2026-06-23T10-00-00.log`（时间戳格式）
2. 创建新 `app.log` 继续写入
3. 旧文件超过 `MaxBackups` 数量时删除最老的
4. `Compress=true` 时旧文件 gzip 压缩为 `.log.gz`

## 依赖

- `gopkg.in/natefinch/lumberjack.v2 v2.2.1`（最成熟的 Go 日志轮转库）

## 集成

- **log.Writer 接口**：直接传给 `log.NewLogger(w)` 即可
- **与 slog/zap 协同**：本插件不依赖任何日志框架，仅实现 zeus 的 `log.Writer`
  - 如需 zap + file_rotate 组合：用 zap 的 `zapcore.AddSync(lumberjack.Logger)` 模式
- **cluster/baggage 自动注入**：`log.Logger.Log` 自动从 ctx 提取并作为 Field 传入，本 writer 直接透传到 JSON

## 性能

- 单条日志单次 `json.Marshal` + 一次 `Write`（lumberjack 内部 buffered I/O）
- 并发安全：用 `sync.Mutex` 保护 marshal+write 原子性（避免多 goroutine 输出交错）
- 高吞吐场景（>10k logs/s）建议：
  - 用 zap writer + lumberjack（zap 内置 buffer，吞吐更高）
  - 或用 filebeat/promtail sidecar 收集文件 → 远程聚合

## 限制

- 不支持按天/小时切分（lumberjack 仅按 size；如需按天切分用 logrotate 系统工具）
- 不支持异步缓冲（业务侧按需 wrap channel）
- 不实现 ELK/Loki 直推（用 filebeat/promtail 收集文件即可）
