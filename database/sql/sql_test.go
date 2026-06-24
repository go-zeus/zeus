package sql

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/trace"
)

// —— spy tracer 实现 ——

type spySpan struct {
	name   string
	attrs  map[string]string
	mu     sync.Mutex
	ended  bool
	err    error
	recErr bool
}

func (s *spySpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}
func (s *spySpan) SetAttributes(attrs map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attrs == nil {
		s.attrs = map[string]string{}
	}
	for k, v := range attrs {
		s.attrs[k] = v
	}
}
func (s *spySpan) SetName(n string) { s.name = n }
func (s *spySpan) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
	s.recErr = true
}
func (s *spySpan) IsRecording() bool { return true }

type spyTracer struct {
	mu    sync.Mutex
	spans []*spySpan
}

func newSpyTracer() *spyTracer { return &spyTracer{} }

func (t *spyTracer) StartSpan(_ context.Context, name string, opts ...trace.SpanOption) (context.Context, trace.Span) {
	cfg := &trace.SpanConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	s := &spySpan{name: name, attrs: cfg.Attrs}
	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return context.Background(), s
}

func (t *spyTracer) Close() error { return nil }

func (t *spyTracer) snapshot() []*spySpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*spySpan, len(t.spans))
	copy(out, t.spans)
	return out
}

// findSpan 按 name 找第一个匹配的 span
func (t *spyTracer) findSpan(name string) *spySpan {
	for _, s := range t.snapshot() {
		if s.name == name {
			return s
		}
	}
	return nil
}

// countByName 统计指定 name 的 span 数量
func (t *spyTracer) countByName(name string) int {
	n := 0
	for _, s := range t.snapshot() {
		if s.name == name {
			n++
		}
	}
	return n
}

// —— spy meter 实现 ——

type spyCounter struct {
	name   string
	labels map[string]string
	mu     sync.Mutex
	value  float64
}

func (c *spyCounter) Inc()          { c.mu.Lock(); c.value++; c.mu.Unlock() }
func (c *spyCounter) Add(v float64) { c.mu.Lock(); c.value += v; c.mu.Unlock() }

type spyHistogram struct {
	name         string
	labels       map[string]string
	mu           sync.Mutex
	observations []float64
}

func (h *spyHistogram) Observe(v float64) {
	h.mu.Lock()
	h.observations = append(h.observations, v)
	h.mu.Unlock()
}

type spyMeter struct {
	mu         sync.Mutex
	counters   []*spyCounter
	histograms []*spyHistogram
}

func newSpyMeter() *spyMeter { return &spyMeter{} }

func (m *spyMeter) Counter(name string, labels map[string]string) metrics.Counter {
	c := &spyCounter{name: name, labels: copyLabels(labels)}
	m.mu.Lock()
	m.counters = append(m.counters, c)
	m.mu.Unlock()
	return c
}

func (m *spyMeter) Histogram(name string, labels map[string]string) metrics.Histogram {
	h := &spyHistogram{name: name, labels: copyLabels(labels)}
	m.mu.Lock()
	m.histograms = append(m.histograms, h)
	m.mu.Unlock()
	return h
}

func (m *spyMeter) Gauge(string, map[string]string) metrics.Gauge { return nil }
func (m *spyMeter) Close() error                                  { return nil }

// countersByName 按 name 查找所有 counter（同一 name 可能多份，因 labels 不同）
func (m *spyMeter) countersByName(name string) []*spyCounter {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*spyCounter
	for _, c := range m.counters {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

// counterTotal 按 name 累加所有 counter value
func (m *spyMeter) counterTotal(name string) float64 {
	var sum float64
	for _, c := range m.countersByName(name) {
		c.mu.Lock()
		sum += c.value
		c.mu.Unlock()
	}
	return sum
}

// histogramsByName 按 name 查找所有 histogram
func (m *spyMeter) histogramsByName(name string) []*spyHistogram {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*spyHistogram
	for _, h := range m.histograms {
		if h.name == name {
			out = append(out, h)
		}
	}
	return out
}

func copyLabels(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// —— 测试用例 ——

func TestMain(m *testing.M) {
	registerFakeDriver()
	m.Run()
}

// TestNew_InvalidDriver 驱动名缺失时报错
func TestNew_InvalidDriver(t *testing.T) {
	_, err := New(database.DBOptions{}, nil, nil)
	if err == nil {
		t.Fatal("expected error when Driver is empty")
	}
}

// TestNew_BadDSN DSN 错误时 sql.Open 成功但 Ping 失败
//
// 注意：sql.Open 不实际连，需要 Ping 才能验证 DSN
func TestNew_BadDSN(t *testing.T) {
	db, err := New(database.DBOptions{
		Driver: fakeDriverName,
		DSN:    "error", // fake driver 在 Open 时报错
	}, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	if err := db.Ping(context.Background()); err == nil {
		t.Error("Ping with bad DSN should error")
	}
}

// TestNew_DefaultsNoTracerMeter tracer/meter 为 nil 时用 noop
func TestNew_DefaultsNoTracerMeter(t *testing.T) {
	db, err := New(database.DBOptions{
		Driver: fakeDriverName,
		DSN:    "ok",
	}, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(context.Background(), "INSERT"); err != nil {
		t.Errorf("Exec with noop tracer/meter failed: %v", err)
	}
}

// TestExec_Success 正常执行 + 自动 metric 上报 + span 创建
func TestExec_Success(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()

	db, err := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	res, err := db.Exec(context.Background(), "INSERT INTO users(name) VALUES(?)", "alice")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		t.Errorf("RowsAffected = %d, want 1", affected)
	}

	// 验证 metric 上报
	if got := meter.counterTotal(metricQueryTotal); got != 1 {
		t.Errorf("counter total = %v, want 1", got)
	}
	// 验证 span 创建 + attrs 正确
	span := tracer.findSpan("db.exec")
	if span == nil {
		t.Fatal("span db.exec not found")
	}
	if span.attrs["query"] != "INSERT INTO users(name) VALUES(?)" {
		t.Errorf("span attrs[query] = %q", span.attrs["query"])
	}
	if span.attrs["db"] != fakeDriverName {
		t.Errorf("span attrs[db] = %q, want %s", span.attrs["db"], fakeDriverName)
	}
	if _, ok := span.attrs["tx_id"]; !ok {
		t.Error("span attrs missing tx_id")
	}
}

// TestQuery_Success 正常查询 + Rows 迭代
func TestQuery_Success(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()

	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	defer db.Close()

	rows, err := db.Query(context.Background(), "SELECT id, name FROM users")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	var (
		ids   []int64
		names []string
	)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		ids = append(ids, id)
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("ids = %v, want [1 2]", ids)
	}
	if len(names) != 2 || names[0] != "alice" || names[1] != "bob" {
		t.Errorf("names = %v, want [alice bob]", names)
	}

	// 验证 span
	if span := tracer.findSpan("db.query"); span == nil {
		t.Error("span db.query not found")
	}
}

// TestQueryRow_Success 单行查询
func TestQueryRow_Success(t *testing.T) {
	drv.resetCounters()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil)
	defer db.Close()

	var id int64
	var name string
	err := db.QueryRow(context.Background(), "SELECT id, name FROM users LIMIT 1").Scan(&id, &name)
	if err != nil {
		t.Fatalf("QueryRow.Scan: %v", err)
	}
	if id != 1 || name != "alice" {
		t.Errorf("got (%d,%q), want (1,alice)", id, name)
	}
}

// TestExec_Error 错误路径：metric 打 status=error + span 记 RecordError
func TestExec_Error(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()

	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	defer db.Close()

	_, err := db.Exec(context.Background(), "error")
	if err == nil {
		t.Fatal("expected error for query=\"error\"")
	}

	// metric 应该有 status=error 计数
	c := meter.countersByName(metricQueryTotal)
	foundErr := false
	for _, counter := range c {
		if counter.labels["status"] == "error" {
			counter.mu.Lock()
			if counter.value > 0 {
				foundErr = true
			}
			counter.mu.Unlock()
		}
	}
	if !foundErr {
		t.Error("no metric counter with status=error")
	}

	// span 应该有 RecordError 调用
	span := tracer.findSpan("db.exec")
	if span == nil {
		t.Fatal("span db.exec not found")
	}
	if !span.recErr {
		t.Error("span.RecordError not called")
	}
}

// TestBeginTx_Lifecycle 完整事务：Begin → Exec → Commit
func TestBeginTx_Lifecycle(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()

	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	defer db.Close()

	tx, err := db.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if _, err := tx.Exec(context.Background(), "INSERT"); err != nil {
		t.Fatalf("tx.Exec: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// 验证 span：应有 begin + exec + commit 三个
	if got := tracer.countByName("db.tx.begin"); got != 1 {
		t.Errorf("span db.tx.begin count = %d, want 1", got)
	}
	if got := tracer.countByName("db.tx.exec"); got != 1 {
		t.Errorf("span db.tx.exec count = %d, want 1", got)
	}
	if got := tracer.countByName("db.tx.commit"); got != 1 {
		t.Errorf("span db.tx.commit count = %d, want 1", got)
	}
}

// TestBeginTx_Rollback Rollback 路径
func TestBeginTx_Rollback(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil)
	defer db.Close()

	tx, err := db.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if got := tracer.countByName("db.tx.rollback"); got != 1 {
		t.Errorf("span db.tx.rollback count = %d, want 1", got)
	}
}

// TestPing 透传 Ping 到底层 *sql.DB
func TestPing(t *testing.T) {
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil)
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// TestEnsureTxID_AutoGenerated Exec 自动生成 tx_id 注入 baggage
//
// 关键契约：用户不显式调 WithTxID，框架在 Exec 入口自动生成 + 注入
func TestEnsureTxID_AutoGenerated(t *testing.T) {
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil)
	defer db.Close()

	ctx := context.Background()
	if id := database.TxIDFromContext(ctx); id != "" {
		t.Fatalf("precondition: ctx already has tx_id %q", id)
	}

	_, _ = db.Exec(ctx, "INSERT")

	// Exec 调用后，ctx 应该不变（EnsureTxID 返回新 ctx），但原 ctx 仍然没 tx_id
	// 验证方式：用 spy tracer 检查 span attrs 有 tx_id
	tracer := newSpyTracer()
	db2, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil)
	defer db2.Close()
	_, _ = db2.Exec(context.Background(), "INSERT")
	span := tracer.findSpan("db.exec")
	if span == nil || span.attrs["tx_id"] == "" {
		t.Error("span missing tx_id attr")
	}
}

// TestEndToEnd_TxIDPropagation tx_id 跨操作传播 + 通过 propagation 透传
//
// 场景：用户在业务层注入 tx_id → DB 操作 span 携带同一 tx_id
func TestEndToEnd_TxIDPropagation(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil)
	defer db.Close()

	// 业务层注入 tx_id
	ctx := database.WithTxID(context.Background(), "tx-business-001")

	// 执行多个操作
	_, _ = db.Exec(ctx, "INSERT")
	_, _ = db.Exec(ctx, "UPDATE")

	// 所有 span 的 tx_id 应该都是 "tx-business-001"
	spans := tracer.snapshot()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	for _, s := range spans {
		if s.attrs["tx_id"] != "tx-business-001" {
			t.Errorf("span %s attrs[tx_id] = %q, want tx-business-001", s.name, s.attrs["tx_id"])
		}
	}

	// 同时 ctx baggage 应该携带 tx_id（通过 propagation）
	if v, ok := propagation.Get(ctx, database.TxIDKey); !ok || v != "tx-business-001" {
		t.Errorf("propagation.Get(%q) = (%q,%v)", database.TxIDKey, v, ok)
	}
}

// TestTx_TxIDShared 事务内所有操作共享同一 tx_id
//
// 关键契约：BeginTx 时生成的 tx_id 在 tx.Exec/Commit 时保持一致
func TestTx_TxIDShared(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil)
	defer db.Close()

	// 不显式注入 tx_id，让 BeginTx 自动生成
	tx, _ := db.BeginTx(context.Background())
	_, _ = tx.Exec(context.Background(), "INSERT")
	_ = tx.Commit()

	spans := tracer.snapshot()
	// 找到 begin 的 tx_id 作为基准
	var beginTxID string
	for _, s := range spans {
		if s.name == "db.tx.begin" {
			beginTxID = s.attrs["tx_id"]
		}
	}
	if beginTxID == "" {
		t.Fatal("begin span missing tx_id")
	}

	// 所有 tx 操作的 tx_id 必须一致
	for _, s := range spans {
		if got := s.attrs["tx_id"]; got != beginTxID {
			t.Errorf("span %s tx_id = %q, want %q", s.name, got, beginTxID)
		}
	}
}

// TestExec_Concurrent 并发 100 个 Exec，counter 累加正确
func TestExec_Concurrent(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	defer db.Close()

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = db.Exec(context.Background(), "INSERT")
		}()
	}
	wg.Wait()

	if got := meter.counterTotal(metricQueryTotal); got != N {
		t.Errorf("counter total = %v, want %d", got, N)
	}
	if got := len(tracer.snapshot()); got != N {
		t.Errorf("span count = %d, want %d", got, N)
	}
}

// TestOptions_WithName 自定义 db 名用于 metric label
func TestOptions_WithName(t *testing.T) {
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil, WithName("user-db"))
	defer db.Close()

	_, _ = db.Exec(context.Background(), "INSERT")

	span := tracer.findSpan("db.exec")
	if span.attrs["db"] != "user-db" {
		t.Errorf("attrs[db] = %q, want user-db", span.attrs["db"])
	}
}

// TestRows_CloseBeforeNext 提前 Close 时 Next 返回错误
func TestRows_CloseBeforeNext(t *testing.T) {
	drv.resetCounters()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil)
	defer db.Close()

	rows, _ := db.Query(context.Background(), "SELECT id, name FROM users")
	if err := rows.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Close 后 Next 应返回 false
	if rows.Next() {
		t.Error("Next after Close should return false")
	}
}

// TestBeginTx_WithOptions 隔离级别 + 只读选项（fake driver 不校验，仅验证不 panic）
func TestBeginTx_WithOptions(t *testing.T) {
	drv.resetCounters()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil)
	defer db.Close()

	tx, err := db.BeginTx(context.Background(),
		database.WithReadOnly(true),
		database.WithIsolation(5), // serializable
	)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	_ = tx.Commit()
}

// TestExec_DurationHistogramObserve histogram 被调用并观测到正值
func TestExec_DurationHistogramObserve(t *testing.T) {
	drv.resetCounters()
	meter := newSpyMeter()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, meter)
	defer db.Close()

	_, _ = db.Exec(context.Background(), "INSERT")

	hs := meter.histogramsByName(metricQueryDuration)
	if len(hs) == 0 {
		t.Fatal("no histogram recorded")
	}
	hs[0].mu.Lock()
	defer hs[0].mu.Unlock()
	if len(hs[0].observations) != 1 || hs[0].observations[0] <= 0 {
		t.Errorf("observations = %v, want one positive value", hs[0].observations)
	}
}

// ExampleNew 简单使用示例
func ExampleNew() {
	registerFakeDriver()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil)
	defer db.Close()

	rows, _ := db.Query(context.Background(), "SELECT id, name FROM users")
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		_ = rows.Scan(&id, &name)
		fmt.Printf("%d:%s ", id, name)
	}
	// Output: 1:alice 2:bob
}

// TestWithTracer_WithMeter 使用 Option 形式注入 tracer/meter（覆盖 nil 不覆盖分支）
func TestWithTracer_WithMeter(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()
	// New 时不传 tracer/meter，通过 Option 注入
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, nil,
		WithTracer(tracer), WithMeter(meter), WithName("opt-db"),
	)
	defer db.Close()

	_, _ = db.Exec(context.Background(), "INSERT")

	if span := tracer.findSpan("db.exec"); span == nil {
		t.Error("WithTracer 注入后应记录 span")
	}
	if cs := meter.countersByName(metricQueryTotal); len(cs) == 0 {
		t.Error("WithMeter 注入后应记录 counter")
	}
}

// TestWithTracer_NilIgnored 传 nil 不应 panic 也不应清空已有 tracer
func TestWithTracer_NilIgnored(t *testing.T) {
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil,
		WithTracer(nil), // 应被忽略
	)
	defer db.Close()
	_, _ = db.Exec(context.Background(), "INSERT")
	if got := tracer.countByName("db.exec"); got != 1 {
		t.Errorf("nil WithTracer should not clear existing tracer, got span count = %d", got)
	}
}

// TestWithMeter_NilIgnored 同上
func TestWithMeter_NilIgnored(t *testing.T) {
	meter := newSpyMeter()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, nil, meter,
		WithMeter(nil),
	)
	defer db.Close()
	_, _ = db.Exec(context.Background(), "INSERT")
	if cs := meter.countersByName(metricQueryTotal); len(cs) == 0 {
		t.Error("nil WithMeter should not clear existing meter")
	}
}

// TestTx_Query 事务内 Query 路径
func TestTx_Query(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	defer db.Close()

	tx, err := db.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	rows, err := tx.Query(context.Background(), "SELECT id FROM users")
	if err != nil {
		t.Fatalf("tx.Query: %v", err)
	}
	defer rows.Close()
	if got := tracer.countByName("db.tx.query"); got != 1 {
		t.Errorf("span db.tx.query count = %d, want 1", got)
	}
	_ = tx.Commit()
}

// TestTx_QueryRow 事务内 QueryRow 路径
func TestTx_QueryRow(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil)
	defer db.Close()

	tx, err := db.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	var id int64
	var name string
	if err := tx.QueryRow(context.Background(), "SELECT id, name FROM users LIMIT 1").Scan(&id, &name); err != nil {
		t.Fatalf("tx.QueryRow.Scan: %v", err)
	}
	if id != 1 || name != "alice" {
		t.Errorf("got (%d,%q), want (1,alice)", id, name)
	}
	_ = tx.Commit()
}

// TestTx_Query_Error 事务 Query 返回错误路径
func TestTx_Query_Error(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, nil)
	defer db.Close()

	tx, _ := db.BeginTx(context.Background())
	_, err := tx.Query(context.Background(), "error")
	if err == nil {
		t.Error("expected error for query=\"error\"")
	}
	if got := tracer.countByName("db.tx.query"); got != 1 {
		t.Errorf("span db.tx.query count = %d, want 1", got)
	}
	_ = tx.Rollback()
}

// TestTx_Exec_Error 事务 Exec 返回错误路径
func TestTx_Exec_Error(t *testing.T) {
	drv.resetCounters()
	tracer := newSpyTracer()
	meter := newSpyMeter()
	db, _ := New(database.DBOptions{Driver: fakeDriverName, DSN: "ok"}, tracer, meter)
	defer db.Close()

	tx, _ := db.BeginTx(context.Background())
	_, err := tx.Exec(context.Background(), "error")
	if err == nil {
		t.Error("expected error for query=\"error\"")
	}
	// 错误路径应记录 status=error
	foundErr := false
	for _, c := range meter.countersByName(metricQueryTotal) {
		if c.labels["status"] == "error" {
			c.mu.Lock()
			if c.value > 0 {
				foundErr = true
			}
			c.mu.Unlock()
		}
	}
	if !foundErr {
		t.Error("no metric counter with status=error for tx.Exec")
	}
	_ = tx.Rollback()
}

// 防止 time 包未使用 import
var _ = time.Second
var _ = errors.New
