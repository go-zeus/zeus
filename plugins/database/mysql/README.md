# MySQL 数据库插件

`database.DB` 接口的 MySQL 实现，薄包装 `go-sql-driver/mysql` 驱动，复用主包 `database/sql` 的全部 trace / metrics / tx_id 注入能力。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/database/mysql
```

插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "context"

    "github.com/go-zeus/zeus/database"
    "github.com/go-zeus/zeus/plugins/database/mysql"
)

func main() {
    db, err := mysql.New(database.DBOptions{
        DSN:         mysql.BuildDSN("root", "pass", "127.0.0.1", mysql.DefaultPort, "shop"),
        MaxOpenConns: 50,
    }, tracer, meter)
    if err != nil {
        panic(err)
    }
    defer db.Close()

    ctx := context.Background()

    // 建表
    _, _ = db.Exec(ctx,
        `CREATE TABLE IF NOT EXISTS users (
            id   BIGINT PRIMARY KEY AUTO_INCREMENT,
            name VARCHAR(64) NOT NULL,
            age  INT
        )`)

    // 插入
    _, _ = db.Exec(ctx, "INSERT INTO users(name, age) VALUES(?, ?)", "alice", 18)

    // 事务
    tx, _ := db.BeginTx(ctx)
    ctx = database.WithTx(ctx, tx)
    _, _ = tx.Exec(ctx, "UPDATE users SET age = ? WHERE name = ?", 19, "alice")
    _ = tx.Commit()

    // 查询
    rows, _ := db.Query(ctx, "SELECT id, name FROM users WHERE age > ?", 10)
    defer rows.Close()
    for rows.Next() {
        var id int64
        var name string
        _ = rows.Scan(&id, &name)
    }
}
```

## 选项

`mysql.New` 接收 `database.DBOptions`（主包统一定义）：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `DSN` | `string` | 必填 | MySQL 连接串，建议用 `BuildDSN()` 构造 |
| `Driver` | `string` | 强制 `"mysql"` | 用户传入会被覆盖，避免拼错 |
| `MaxOpenConns` | `int` | `0`（不限） | 连接池上限 |
| `MaxIdleConns` | `int` | `0` | 空闲连接数 |
| `ConnMaxLifetime` | `time.Duration` | `0` | 连接最长生命周期 |
| `ConnMaxIdleTime` | `time.Duration` | `0` | 空闲连接最长存活 |

`BuildDSN` 助手签名：

```go
func BuildDSN(user, pass, host string, port int, dbname string) string
```

生成的 DSN 默认追加 `parseTime=true&loc=Local&charset=utf8mb4`，避免时区与 Unicode 常见坑。

## 依赖

- `github.com/go-sql-driver/mysql`（通过 `import _` 副作用注册驱动）

## 集成

- 通过 `mysql.New(opts, tracer, meter)` 注入 Tracer / Meter，每次 Query/Exec 自动埋点：
  - trace span：`db.query` / `db.exec` / `db.tx.begin` / `db.tx.commit` / `db.tx.rollback`，attrs 含 `db` / `query` / `tx_id`
  - metrics：counter `db_query_total{db,op,status}` + histogram `db_query_duration{db,op}`
- 同进程事务传播：`database.WithTx(ctx, tx)` + `database.FromTx(ctx)`
- 跨服务审计：`database.WithTxID(ctx, id)` 通过 propagation 自动透传 tx_id
- URL scheme：`import _ "plugins/database/mysql"` 后可用 `database.NewFromURL("mysql://user:pass@host:port/db?pool=50&lifetime=30m", tracer, meter)`
- tracer / meter 为 nil 时退化为 noop，不影响业务逻辑
- 完整端到端示例参考 `examples/database/`
