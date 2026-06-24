package sql

// 极简 fake driver（仅测试用）
//
// 实现接口：
//   - driver.Driver / driver.DriverContext（可选）
//   - driver.Conn + ExecerContext + QueryerContext + Pinger + ConnBeginTx
//   - driver.Tx
//   - driver.Result
//   - driver.Rows
//
// 不实现：
//   - 真实 SQL 解析（所有 Exec 返回固定 result，所有 Query 返回固定 rows）
//   - Stmt 路径（强制走 ExecerContext/QueryerContext，简化代码）
//
// 用途：验证 wrapper 的 trace/metrics/tx_id 注入路径，不验证 SQL 语义。

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
)

const fakeDriverName = "zeus-fake"

// fakeDriver 测试驱动
type fakeDriver struct {
	mu       sync.Mutex
	execCnt  int
	queryCnt int
}

var (
	drv               = &fakeDriver{}
	registerDriverOnc sync.Once
)

// registerFakeDriver 注册驱动（幂等）
func registerFakeDriver() {
	registerDriverOnc.Do(func() {
		sql.Register(fakeDriverName, drv)
	})
}

// —— driver.Driver ——

// Open 解析 DSN（DSN="error" 时模拟连接失败）
func (d *fakeDriver) Open(dsn string) (sqldriver.Conn, error) {
	if dsn == "error" {
		return nil, errors.New("fake driver: simulated open error")
	}
	return &fakeConn{driver: d}, nil
}

// —— driver.Conn + ExecerContext + QueryerContext + Pinger + ConnBeginTx ——

type fakeConn struct {
	driver *fakeDriver
	closed bool
}

func (c *fakeConn) Prepare(query string) (sqldriver.Stmt, error) {
	return nil, errors.New("fake driver: Prepare not supported")
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func (c *fakeConn) Begin() (sqldriver.Tx, error) {
	return c.beginTx(context.Background(), sqldriver.TxOptions{})
}

// BeginTx 实现 driver.ConnBeginTx
func (c *fakeConn) BeginTx(ctx context.Context, opts sqldriver.TxOptions) (sqldriver.Tx, error) {
	return c.beginTx(ctx, opts)
}

func (c *fakeConn) beginTx(_ context.Context, _ sqldriver.TxOptions) (sqldriver.Tx, error) {
	if c.closed {
		return nil, errors.New("fake driver: conn closed")
	}
	return &fakeTx{conn: c}, nil
}

// Ping 实现 driver.Pinger
func (c *fakeConn) Ping(_ context.Context) error {
	if c.closed {
		return errors.New("fake driver: conn closed")
	}
	return nil
}

// ExecContext 实现 driver.ExecerContext
//
// query="error" 时模拟执行失败；其他情况计数并返回固定 Result
func (c *fakeConn) ExecContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Result, error) {
	if c.closed {
		return nil, errors.New("fake driver: conn closed")
	}
	c.driver.mu.Lock()
	c.driver.execCnt++
	c.driver.mu.Unlock()
	if query == "error" {
		return nil, errors.New("fake driver: simulated exec error")
	}
	return fakeResult{lastID: 1, affected: 1}, nil
}

// QueryContext 实现 driver.QueryerContext
//
// query="error" 时模拟查询失败；其他情况返回固定 2 行（id, name）
func (c *fakeConn) QueryContext(_ context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	if c.closed {
		return nil, errors.New("fake driver: conn closed")
	}
	c.driver.mu.Lock()
	c.driver.queryCnt++
	c.driver.mu.Unlock()
	if query == "error" {
		return nil, errors.New("fake driver: simulated query error")
	}
	return &fakeRows{
		columns: []string{"id", "name"},
		rows: [][]any{
			{int64(1), "alice"},
			{int64(2), "bob"},
		},
	}, nil
}

// —— driver.Tx ——

type fakeTx struct {
	conn   *fakeConn
	closed bool
}

func (t *fakeTx) Commit() error {
	if t.closed {
		return errors.New("fake driver: tx already done")
	}
	t.closed = true
	return nil
}

func (t *fakeTx) Rollback() error {
	if t.closed {
		return errors.New("fake driver: tx already done")
	}
	t.closed = true
	return nil
}

// —— driver.Result ——

type fakeResult struct {
	lastID   int64
	affected int64
}

func (r fakeResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.affected, nil }

// —— driver.Rows ——

type fakeRows struct {
	columns []string
	rows    [][]any
	pos     int
	closed  bool
	mu      sync.Mutex
}

func (r *fakeRows) Columns() []string { return r.columns }

func (r *fakeRows) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

func (r *fakeRows) Next(dest []sqldriver.Value) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("fake driver: rows closed")
	}
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.pos]
	for i := range dest {
		dest[i] = row[i]
	}
	r.pos++
	return nil
}

// resetCounters 测试辅助：重置驱动计数
func (d *fakeDriver) resetCounters() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.execCnt = 0
	d.queryCnt = 0
}

// snapshotCounters 测试辅助：快照当前计数
func (d *fakeDriver) snapshotCounters() (exec, query int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.execCnt, d.queryCnt
}

// driverStats 测试辅助：返回可读字符串
func (d *fakeDriver) stats() string {
	exec, query := d.snapshotCounters()
	return fmt.Sprintf("exec=%d query=%d", exec, query)
}
