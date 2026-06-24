# SQLite 数据库插件

`database.DB` 的 [SQLite](https://www.sqlite.org/) 实现，基于 [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)（纯 Go，无 cgo，跨平台编译零障碍）。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/database/sqlite
```

> 插件是独立 module，不会污染主仓依赖。
> **为何选 modernc.org/sqlite 而非 mattn/go-sqlite3？**：纯 Go 实现，不需要 cgo，CI / Docker 跨平台编译零障碍。

## 使用

### 1. 文件 DB（本地开发场景）

```go
import (
    "github.com/go-zeus/zeus/database"
    "github.com/go-zeus/zeus/plugins/database/sqlite"
)

db, err := sqlite.New(database.DBOptions{
    DSN: sqlite.BuildDSN("test.db", sqlite.OpenReadWriteCreate),
    MaxOpenConns: 1, // SQLite 写并发建议 1（避免 SQLITE_BUSY）
}, tracer, meter)
if err != nil { return err }
defer db.Close()

_, err = db.Exec(ctx, "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)")
```

### 2. 内存 DB（单测场景）

```go
db, err := sqlite.New(database.DBOptions{
    DSN: sqlite.BuildMemoryDSN(false), // 每个连接独立内存 DB
    MaxOpenConns: 1,
}, nil, nil) // tracer/meter 可为 nil（noop）
```

### 3. URL scheme 装配

```go
import _ "github.com/go-zeus/zeus/plugins/database/sqlite"

db, err := database.NewFromURL("sqlite://test.db?pool=1&lifetime=5m", tracer, meter)
// 或内存：
db, err := database.NewFromURL("sqlite://:memory:", tracer, meter)
db, err := database.NewFromURL("sqlite://:memory:?cache=shared", tracer, meter)
```

URL 格式：
- `sqlite://path/to/file.db` — 文件 DB
- `sqlite://:memory:` — 内存 DB（每个连接独立）
- `sqlite://:memory:?cache=shared` — 共享内存 DB
- `sqlite://test.db?pool=1&lifetime=5m` — 带 query 参数

## 默认 PRAGMA

`BuildDSN` 自动注入以下 PRAGMA（最佳实践）：

| PRAGMA | 值 | 说明 |
|--------|-----|------|
| `busy_timeout` | 5000ms | 锁竞争时等 5s 而非立刻 `SQLITE_BUSY` |
| `foreign_keys` | ON | 启用外键约束（SQLite 默认关闭） |
| `journal_mode` | WAL | Write-Ahead Logging，提升读并发 |

内存 DB 用 `journal_mode=MEMORY`（替代 WAL，内存场景更优）。

## 性能建议

| 场景 | MaxOpenConns | 说明 |
|------|--------------|------|
| 仅写 | 1 | 避免 SQLITE_BUSY 锁竞争 |
| 读多写少 | 不限 | WAL 模式下读不阻塞写 |
| 单测 | 1 | 隔离测试 |

## API

### 构造

```go
sqlite.New(opts database.DBOptions, tracer, meter) (database.DB, error)
sqlite.BuildDSN(path string, flag OpenFlag) string
sqlite.BuildMemoryDSN(shared bool) string
```

### 常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `DriverName` | `"sqlite"` | driver 名 |
| `OpenReadOnly` | 1 | 只读 |
| `OpenReadWrite` | 2 | 读写 |
| `OpenReadWriteCreate` | 6 | 读写 + 创建（默认） |

## 依赖

- `modernc.org/sqlite v1.33.1`（纯 Go SQLite 实现）
- 间接：`modernc.org/libc` / `modernc.org/mathutil` 等辅助库

## 集成

- **URL scheme**：`sqlite://` 自动启用（需 `import _ "github.com/go-zeus/zeus/plugins/database/sqlite"`）
- **trace/metrics**：每次 Query/Exec 自动注入（复用主包 `database/sql` 实现）
- **tx_id**：跨服务 tx_id 透传与 MySQL/Postgres 完全一致
- **示例**：参考 `examples/13-database/`（业务代码无需改动即可切换 driver）

## 限制

- 不支持读写分离（SQLite 是单文件 DB，没有主从概念）
- 不支持分布式事务（同进程 `WithTx` 可用，跨服务 tx_id 仅做审计透传）
- modernc.org/sqlite 性能略低于 mattn/go-sqlite3（cgo 版本），但跨平台编译更方便
  - 如必须用 cgo 版本：自行 import `github.com/mattn/go-sqlite3`，并把 `opts.Driver` 改为 `"sqlite3"`
