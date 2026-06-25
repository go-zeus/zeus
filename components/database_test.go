package components

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	stdsql "database/sql"

	"github.com/go-zeus/zeus/database"
)

// mockDB 用于测试 DatabaseComponent 编排（不依赖真实驱动）
type mockDB struct {
	mu          sync.Mutex
	pingCalled  bool
	closeCalled bool
	pingErr     error
	closeErr    error
}

func (m *mockDB) Query(context.Context, string, ...any) (database.Rows, error) { return nil, nil }
func (m *mockDB) QueryRow(context.Context, string, ...any) database.Row        { return nil }
func (m *mockDB) Exec(context.Context, string, ...any) (stdsql.Result, error)  { return nil, nil }
func (m *mockDB) BeginTx(context.Context, ...database.TxOption) (database.Tx, error) {
	return nil, nil
}
func (m *mockDB) Ping(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingCalled = true
	return m.pingErr
}
func (m *mockDB) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return m.closeErr
}

// TestDatabaseComponent_Lifecycle OnStart 调 Ping，OnStop 调 Close
func TestDatabaseComponent_Lifecycle(t *testing.T) {
	mock := &mockDB{}
	dc := NewDatabaseComponent(mock)

	c := NewContainer()
	_ = c.Register(dc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	mock.mu.Lock()
	if !mock.pingCalled {
		t.Error("Ping should be called on Start")
	}
	mock.mu.Unlock()

	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	mock.mu.Lock()
	if !mock.closeCalled {
		t.Error("Close should be called on Stop")
	}
	mock.mu.Unlock()
}

// TestDatabaseComponent_PingError Ping 失败时 Start 返回 error
func TestDatabaseComponent_PingError(t *testing.T) {
	pingErr := errors.New("ping failed")
	mock := &mockDB{pingErr: pingErr}
	dc := NewDatabaseComponent(mock)

	c := NewContainer()
	_ = c.Register(dc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := c.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Ping")
	}
	if !errors.Is(err, pingErr) {
		t.Errorf("err = %v, want %v", err, pingErr)
	}
}

// TestDatabaseComponent_PingDisabled WithPingOnStart(false) 时跳过 Ping
func TestDatabaseComponent_PingDisabled(t *testing.T) {
	mock := &mockDB{}
	dc := NewDatabaseComponent(mock, WithPingOnStart(false))

	c := NewContainer()
	_ = c.Register(dc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	mock.mu.Lock()
	if mock.pingCalled {
		t.Error("Ping should NOT be called when WithPingOnStart(false)")
	}
	mock.mu.Unlock()
}

// TestDatabaseComponent_NilDB DB 为 nil 时 no-op
func TestDatabaseComponent_NilDB(t *testing.T) {
	dc := NewDatabaseComponent(nil)

	c := NewContainer()
	_ = c.Register(dc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Errorf("Start with nil DB should be no-op, got: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop with nil DB should be no-op, got: %v", err)
	}
}

// TestDatabaseComponent_Provide Provide 发布 database.DB
func TestDatabaseComponent_Provide(t *testing.T) {
	mock := &mockDB{}
	dc := NewDatabaseComponent(mock, WithPingOnStart(false))

	c := NewContainer()
	_ = c.Register(dc)

	// 加一个抓取 broker 的辅助组件
	got := make(chan database.DB, 1)
	_ = c.Register(&dbCapturer{capture: got})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case v := <-got:
		if v == nil {
			t.Error("captured DB is nil")
		}
	case <-time.After(time.Second):
		t.Fatal("DB not captured within 1s")
	}
}

// dbCapturer 测试辅助：OnStart 时通过 GetType 抓取 DB
type dbCapturer struct {
	capture chan<- database.DB
}

func (d *dbCapturer) Name() string      { return "db_capturer" }
func (d *dbCapturer) Depends() []string { return []string{"database"} }
func (d *dbCapturer) Provide(_ Context) (any, error) {
	return d, nil
}
func (d *dbCapturer) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			db, err := Type[database.DB](ctx)
			if err != nil {
				return err
			}
			d.capture <- db
			return nil
		},
	}
}
