---
title: 数据库
weight: 70
---

Zeus 提供 `database` 包作为薄封装 stdlib 抽象，**不做 ORM**（用户可自由选 sqlx / gorm / ent / sqlc）。

## 核心概念

| 概念 | 说明 |
|---|---|
| `DB` / `Tx` / `Rows` / `Row` | 薄接口，签名几乎对齐 `*sql.DB`（学习成本零） |
| `DBOptions` | 连接池配置（Driver/DSN/MaxOpenConns/...） |
| `TxOption` | 事务选项（WithIsolation / WithReadOnly） |
| `WithTx`/`FromTx` | 同进程事务传播（多个 Repository 共享 Tx） |
| `WithTxID`/`TxIDFromContext`/`EnsureTxID` | 跨服务 tx_id 传播（审计/排查用，**不做 2PC**） |
| `database/sql` | 内置实现：薄封装 stdlib `*sql.DB` + 自动 trace/metrics/tx_id |
| `DatabaseComponent` | components 适配器：OnStart Ping，OnStop Close |

## 自动集成矩阵

每次 Query/Exec 触发：

| 集成 | 行为 |
|---|---|
| **trace** | span `db.query`/`db.exec`/`db.tx.begin`/`db.tx.commit`/`db.tx.rollback`；attrs: `db`, `query`, `tx_id` |
| **metrics** | counter `db_query_total{db,op,status}` + histogram `db_query_duration{db,op}` |
| **propagation** | `EnsureTxID` 自动写入 `Bag{zeus.tx.id}`，随 client/server 跨服务透传 |

## 使用方式

```go
import (
    "github.com/go-zeus/zeus/database"
    sqldriver "github.com/go-zeus/zeus/database/sql"
    _ "github.com/go-sql-driver/mysql"  // 注册驱动
)

db, _ := sqldriver.New(database.DBOptions{
    Driver: "mysql",
    DSN:    "user:pass@tcp(127.0.0.1:3306)/db",
    MaxOpenConns: 50,
}, tracer, meter)

// 单条操作
rows, _ := db.Query(ctx, "SELECT id, name FROM users WHERE age > ?", 18)

// 事务（同进程 tx 传播 + 自动 tx_id）
tx, _ := db.BeginTx(ctx)
ctx = database.WithTx(ctx, tx)
// repoA / repoB 内部 FromTx(ctx) 优先取事务句柄
_ = tx.Commit()

// 跨服务：tx_id 自动透传到下游（client 自动注入到 Baggage）
ctx = database.WithTxID(ctx, "biz-tx-001")
```

## 接入真实驱动

内置 `database/sql` 已满足单进程零依赖场景。分布式部署走 plugins：

```go
import (
    "github.com/go-zeus/zeus/database"
    mysql "github.com/go-zeus/zeus/plugins/database/mysql"
)

db, err := mysql.New(database.DBOptions{
    DSN:         mysql.BuildDSN("root", "pass", "127.0.0.1", mysql.DefaultPort, "test"),
    MaxOpenConns: 50,
}, tracer, meter)
```

完整示例参见 `examples/database/`：建表/插入/查询/事务/tx_id 透传（用 fake driver，无需真实 DB）。
