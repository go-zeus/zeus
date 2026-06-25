// 极简 fake driver（仅示例用）。
//
// 复制自 database/sql/fakedriver_test.go（简化版），
// 因 examples 是独立 module 无法访问主包 test 文件。
package main

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
)

type exampleFakeDriver struct{}

func (d *exampleFakeDriver) Open(_ string) (driver.Conn, error) {
	return &exampleConn{}, nil
}

type exampleConn struct{}

func (c *exampleConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("fake: Prepare not supported")
}
func (c *exampleConn) Close() error              { return nil }
func (c *exampleConn) Begin() (driver.Tx, error) { return &exampleTx{}, nil }
func (c *exampleConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &exampleTx{}, nil
}
func (c *exampleConn) Ping(context.Context) error { return nil }
func (c *exampleConn) ExecContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return exampleResult{}, nil
}
func (c *exampleConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return &exampleRows{}, nil
}

type exampleTx struct{}

func (t *exampleTx) Commit() error   { return nil }
func (t *exampleTx) Rollback() error { return nil }

type exampleResult struct{}

func (exampleResult) LastInsertId() (int64, error) { return 1, nil }
func (exampleResult) RowsAffected() (int64, error) { return 1, nil }

type exampleRows struct {
	pos    int
	closed bool
}

func (r *exampleRows) Columns() []string { return []string{"id", "name"} }
func (r *exampleRows) Close() error      { r.closed = true; return nil }
func (r *exampleRows) Next(dest []driver.Value) error {
	if r.pos >= 2 {
		return io.EOF
	}
	dest[0] = int64(r.pos + 1)
	dest[1] = "alice"
	r.pos++
	return nil
}
