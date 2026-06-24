// Example: database
//
// 演示声明式数据库接入：
//   - 使用 fake driver（zeus-fake）模拟真实驱动行为，无需 MySQL/Postgres
//   - 通过 components 自动装配：Ping → 优雅关闭 全部自动化
//   - 演示：建表 / 插入 / 查询 / 事务（BeginTx + Commit）
//   - 演示：tx_id 全链路传播（自动注入 ctx baggage）
//
// 启动：
//
//	go run .
//
// 预期输出：
//
//	[INFO] database connected (ping ok)
//	[INFO] exec: CREATE TABLE users (id INT, name VARCHAR(255))
//	[INFO] exec: INSERT INTO users (id, name) VALUES (1, 'alice')
//	[INFO] query: SELECT id, name FROM users → 2 rows
//	[INFO] tx: BEGIN → INSERT → COMMIT (tx_id=...)
//	[INFO] database closed
//
// 真实场景替换：
//
//	import (
//	    sqldriver "github.com/go-zeus/zeus/database/sql"
//	    _ "github.com/go-sql-driver/mysql"
//	)
//	db, _ := sqldriver.New(database.DBOptions{
//	    Driver: "mysql",
//	    DSN:    "user:pass@tcp(127.0.0.1:3306)/test",
//	}, tracer, meter)
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/database"
	sqldriver "github.com/go-zeus/zeus/database/sql"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"
)

// 注册 fake driver（仅示例用；生产代码应 import 真实驱动如 mysql）
//
// 注：fake driver 定义在 database/sql 包的 test 文件中，这里重新定义一份精简版
// 用于 examples 独立编译（examples 是独立 module，无法访问主包 internal test）。

// fakeDriverName fake 驱动名
const fakeDriverName = "zeus-fake-example"

func main() {
	sql.Register(fakeDriverName, &exampleFakeDriver{})
	time.Sleep(0) // 占位避免 import 未使用

	db, err := sqldriver.New(database.DBOptions{
		Driver: fakeDriverName,
		DSN:    "example",
	}, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}

	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewDatabaseComponent(db),
	)

	log.Info("database demo starting；Ctrl+C to stop")

	// 在主 goroutine 中演示 DB 操作（独立于 app.Run 的信号监听）
	go runDemo(db)

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}

// runDemo 演示典型 DB 操作链路
func runDemo(db database.DB) {
	time.Sleep(100 * time.Millisecond) // 等组件启动

	ctx := context.Background()

	// 1) DDL
	_, _ = db.Exec(ctx, "CREATE TABLE users (id INT, name VARCHAR(255))")
	log.Info("exec: CREATE TABLE users (id INT, name VARCHAR(255))")

	// 2) Insert
	_, _ = db.Exec(ctx, "INSERT INTO users (id, name) VALUES (1, 'alice')")
	log.Info("exec: INSERT INTO users (id, name) VALUES (1, 'alice')")

	// 3) Query
	rows, _ := db.Query(ctx, "SELECT id, name FROM users")
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id int64
		var name string
		_ = rows.Scan(&id, &name)
		n++
	}
	log.Info("query: SELECT id, name FROM users → %d rows", n)

	// 4) Transaction（多语句原子化）
	tx, _ := db.BeginTx(ctx)
	_, _ = tx.Exec(ctx, "INSERT INTO users (id, name) VALUES (2, 'bob')")
	_, _ = tx.Exec(ctx, "UPDATE users SET name='alice2' WHERE id=1")
	_ = tx.Commit()

	// 演示 tx_id 全链路传播：业务层注入的 tx_id 自动出现在所有 DB 操作
	ctxWithTxID := database.WithTxID(ctx, "demo-tx-001")
	_, _ = db.Exec(ctxWithTxID, "DELETE FROM users")
	log.Info("exec: DELETE FROM users (tx_id=demo-tx-001)")
	log.Info("tx: BEGIN → INSERT → UPDATE → COMMIT (tx_id 自动生成)")

	log.Info("database demo done；Ctrl+C 退出")
}
