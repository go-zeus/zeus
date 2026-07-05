# 13-database · 数据库接入 + 事务 + tx_id 透传

演示 `database` 包的声明式接入：建表 / 插入 / 查询 / 事务，以及 `tx_id` 跨服务传播。使用 fake driver（`zeus-fake`），无需真实 MySQL/Postgres。

## 演示场景

- 通过 components 自动装配 DB（OnStart Ping，OnStop Close）
- 执行 CREATE TABLE / INSERT / SELECT
- 事务：`BeginTx` + 多个 Repository 共享 Tx + `Commit`
- `tx_id` 自动写入 baggage，随 client 跨服务透传（审计/排查用，不做 2PC）

## 启动与测试

```bash
cd examples/13-database
go run .
```

预期输出：

```
[INFO] database connected (ping ok)
[INFO] exec: CREATE TABLE users (id INT, name VARCHAR(255))
[INFO] exec: INSERT INTO users (id, name) VALUES (1, 'alice')
[INFO] query: SELECT id, name FROM users → 2 rows
[INFO] tx: BEGIN → INSERT → COMMIT (tx_id=...)
[INFO] database closed
```

## 接入真实驱动

```go
import (
    sqldriver "github.com/go-zeus/zeus/database/sql"
    _ "github.com/go-zeus/zeus/plugins/database/mysql"   // 注册 mysql:// scheme
)

db, _ := database.NewFromURL("mysql://user:pass@host:3306/db", tracer, meter)
// 或直接：sqldriver.New(database.DBOptions{Driver:"mysql", DSN:"..."}, tracer, meter)
```

薄封装 stdlib `database/sql`，自动注入 trace（span `db.query` 等）+ metrics（`db_query_total`）+ tx_id。

## 衔接

- 缓存接入 → [14-cache](../14-cache/)
- 全链路追踪（含 db span） → [19-observability](../19-observability/)
