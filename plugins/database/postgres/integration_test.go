package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-zeus/zeus/database"
)

// —— 集成测试（需要真实 PostgreSQL） ——

// skipIfNoPostgres 当 PG 不可达时跳过测试，避免 CI 因环境失败
//
// 探活逻辑：sql.Open 不实际建连，需 Ping 确认
// 返回值：可用 DSN（便于后续测试用例使用）
func skipIfNoPostgres(t *testing.T) (dsn string) {
	t.Helper()
	dsn = os.Getenv("ZEUS_POSTGRES_DSN")
	if dsn == "" {
		dsn = "host=127.0.0.1 port=5432 user=postgres password=postgres dbname=test sslmode=disable connect_timeout=2"
	}
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("postgres 不可达，跳过集成测试 (open failed): %v", err)
		return
	}
	defer func() { _ = raw.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		t.Skipf("postgres 不可达，跳过集成测试 (ping failed): %v", err)
		return
	}
	return dsn
}

// TestPostgres_EndToEnd 端到端：建表 → 插入 → 查询 → 事务
//
// 注意：PG 占位符是 $1/$2，不是 ?
func TestPostgres_EndToEnd(t *testing.T) {
	dsn := skipIfNoPostgres(t)

	db, err := New(database.DBOptions{
		DSN: dsn,
	}, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 0) Ping
	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	// 1) DDL（PG 用 $$ 避免重复创建）
	tableName := fmt.Sprintf("zeus_test_%d", time.Now().UnixNano())
	_, err = db.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, name TEXT)", tableName))
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	defer func() {
		_, _ = db.Exec(ctx, fmt.Sprintf("DROP TABLE %s", tableName))
	}()

	// 2) Insert（PG 占位符 $1/$2）
	_, err = db.Exec(ctx, fmt.Sprintf("INSERT INTO %s (id, name) VALUES ($1, $2)", tableName), 1, "alice")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// 3) Query
	rows, err := db.Query(ctx, fmt.Sprintf("SELECT id, name FROM %s WHERE id = $1", tableName), 1)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if id != 1 || name != "alice" {
			t.Errorf("got (%d,%s), want (1,alice)", id, name)
		}
		n++
	}
	if n != 1 {
		t.Errorf("rows count = %d, want 1", n)
	}

	// 4) Transaction
	tx, err := db.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("INSERT INTO %s (id, name) VALUES ($1, $2)", tableName), 2, "bob"); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx.INSERT: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// 验证事务提交成功
	var count int
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}
