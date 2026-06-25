package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-zeus/zeus/database"
)

// TestBuildDSN_DefaultPort port=0 时使用 DefaultPort
func TestBuildDSN_DefaultPort(t *testing.T) {
	got := BuildDSN("root", "pass", "127.0.0.1", 0, "test")
	want := "root:pass@tcp(127.0.0.1:3306)/test?" + defaultDSNParams
	if got != want {
		t.Errorf("BuildDSN() = %q, want %q", got, want)
	}
}

// TestBuildDSN_CustomPort 自定义端口生效
func TestBuildDSN_CustomPort(t *testing.T) {
	got := BuildDSN("root", "pass", "127.0.0.1", 13306, "test")
	want := "root:pass@tcp(127.0.0.1:13306)/test?" + defaultDSNParams
	if got != want {
		t.Errorf("BuildDSN() = %q, want %q", got, want)
	}
}

// TestBuildDSN_EmptyDBName 空数据库名合法
func TestBuildDSN_EmptyDBName(t *testing.T) {
	got := BuildDSN("root", "", "127.0.0.1", DefaultPort, "")
	want := "root:@tcp(127.0.0.1:3306)/?" + defaultDSNParams
	if got != want {
		t.Errorf("BuildDSN() = %q, want %q", got, want)
	}
}

// TestNew_MissingDSN DSN 为空时返回错误
func TestNew_MissingDSN(t *testing.T) {
	_, err := New(database.DBOptions{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
	if err.Error() == "" {
		t.Errorf("error message should not be empty")
	}
}

// TestNew_ForceMysqlDriver 强制 Driver 覆盖（即使传入错误值）
func TestNew_ForceMysqlDriver(t *testing.T) {
	// 注意：sql.Open 在 Open 阶段不验证 driver 名（只在 Ping 时验证）
	// 这里仅验证 New 本身不报错（Driver 不为空）
	db, err := New(database.DBOptions{
		Driver: "wrong-driver", // 应被强制覆盖为 mysql
		DSN:    "root:@tcp(127.0.0.1:3306)/test",
	}, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()
	// 不 Ping（无 MySQL 实例）；验证 db 实例可用即可
	// 因为底层 sql.Open 用了 "mysql" driver，可以正常创建
	t.Log("New succeeded with forced driver override")
}

// —— 集成测试（需要真实 MySQL） ——

// skipIfNoMySQL 当 MySQL 不可达时跳过测试，避免 CI 因环境失败
//
// 探活逻辑：sql.Open 不实际建连，需 Ping 确认
// 返回值：可用 DSN（便于后续测试用例使用）
func skipIfNoMySQL(t *testing.T) (dsn string) {
	t.Helper()
	dsn = os.Getenv("ZEUS_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/test?parseTime=true"
	}
	raw, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("mysql 不可达，跳过集成测试 (open failed): %v", err)
		return
	}
	defer func() { _ = raw.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		t.Skipf("mysql 不可达，跳过集成测试 (ping failed): %v", err)
		return
	}
	return dsn
}

// TestMySQL_EndToEnd 端到端：建表 → 插入 → 查询 → 事务
func TestMySQL_EndToEnd(t *testing.T) {
	dsn := skipIfNoMySQL(t)

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

	// 1) DDL
	tableName := fmt.Sprintf("zeus_test_%d", time.Now().UnixNano())
	_, err = db.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, name VARCHAR(64))", tableName))
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	defer func() {
		_, _ = db.Exec(ctx, fmt.Sprintf("DROP TABLE %s", tableName))
	}()

	// 2) Insert
	_, err = db.Exec(ctx, fmt.Sprintf("INSERT INTO %s (id, name) VALUES (?, ?)", tableName), 1, "alice")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// 3) Query
	rows, err := db.Query(ctx, fmt.Sprintf("SELECT id, name FROM %s WHERE id = ?", tableName), 1)
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
	if _, err := tx.Exec(ctx, fmt.Sprintf("INSERT INTO %s (id, name) VALUES (?, ?)", tableName), 2, "bob"); err != nil {
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
