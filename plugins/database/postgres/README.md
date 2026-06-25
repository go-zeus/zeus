# PostgreSQL 数据库插件

`database.DB` 接口的 PostgreSQL 实现，副作用注册 `jackc/pgx/v5/stdlib` 驱动，复用主包 `database/sql` 的全部 trace / metrics / tx_id 注入能力。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/database/postgres
```

插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "context"

    "github.com/go-zeus/zeus/database"
    "github.com/go-zeus/zeus/plugins/database/postgres"
)

func main() {
    db, err := postgres.New(database.DBOptions{
        DSN: postgres.BuildDSN("postgres", "pass", "127.0.0.1", postgres.DefaultPort, "shop"),
        MaxOpenConns: 50,
    }, tracer, meter)
    if err != nil {
        panic(err)
    }
    defer db.Close()

    ctx := context.Background()

    // 建表（注意 SERIAL 自增 + $1/$2 占位符）
    _, _ = db.Exec(ctx,
        `CREATE TABLE IF NOT EXISTS users (
            id   BIGSERIAL PRIMARY KEY,
            name VARCHAR(64) NOT NULL,
            age  INT
        )`)

    // 插入（PG 用 $N 占位符，不是 ?）
    _, _ = db.Exec(ctx, "INSERT INTO users(name, age) VALUES($1, $2)", "alice", 18)

    // 事务
    tx, _ := db.BeginTx(ctx)
    ctx = database.WithTx(ctx, tx)
    _, _ = tx.Exec(ctx, "UPDATE users SET age = $1 WHERE name = $2", 19, "alice")
    _ = tx.Commit()

    // 查询
    rows, _ := db.Query(ctx, "SELECT id, name FROM users WHERE age > $1", 10)
    defer rows.Close()
    for rows.Next() {
        var id int64
        var name string
        _ = rows.Scan(&id, &name)
    }
}
```

## DSN 格式

`BuildDSN` 生成 libpq 关键字格式：

```
host=H port=P user=U password=P dbname=D sslmode=disable connect_timeout=10
```

- `port == 0` 时使用 `DefaultPort`（5432）
- `dbname` / `password` 可为空
- 默认 `sslmode=disable`（容器内网部署场景；生产建议改 `require` 或 `verify-full`）

切换 SSL 模式：

```go
postgres.BuildDSNWithSSL("u", "p", "db.example", 5432, "app", "require")
```

## 选项

`postgres.New` 接收 `database.DBOptions`（主包统一定义）：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `DSN` | `string` | 必填 | libpq DSN，建议用 `BuildDSN()` 构造 |
| `Driver` | `string` | 强制 `"pgx"` | 用户传入会被覆盖 |
| `MaxOpenConns` | `int` | `0`（不限） | 连接池上限 |
| `MaxIdleConns` | `int` | `0` | 空闲连接数 |
| `ConnMaxLifetime` | `time.Duration` | `0` | 连接最长生命周期 |
| `ConnMaxIdleTime` | `time.Duration` | `0` | 空闲连接最长存活 |

## 依赖

- `github.com/jackc/pgx/v5`（stdlib adapter 模式，保留 `database/sql` 接口兼容）

## 集成

- 通过 `postgres.New(opts, tracer, meter)` 注入 Tracer / Meter，每次 Query/Exec 自动埋点：
  - trace span：`db.query` / `db.exec` / `db.tx.*`，attrs 含 `db` / `query` / `tx_id`
  - metrics：counter `db_query_total{db,op,status}` + histogram `db_query_duration{db,op}`
- 占位符差异：PG 用 `$1/$2/$3`，从 MySQL 迁移需改写
- 同进程事务传播：`database.WithTx(ctx, tx)` + `database.FromTx(ctx)`
- 跨服务审计：`database.WithTxID(ctx, id)` 通过 propagation 自动透传
- URL scheme：`import _ "plugins/database/postgres"` 后可用 `database.NewFromURL("postgres://user:pass@host:port/db?pool=50&sslmode=require", tracer, meter)`
- 完整端到端示例参考 `examples/database/`
