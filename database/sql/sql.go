// Package sql 提供 stdlib database/sql 的薄封装实现。
//
// 设计目的：
//   - 自动注入 trace span（每次 Query/Exec 创建 span）
//   - 自动上报 metrics（counter + histogram）
//   - 自动注入 tx_id 到 ctx baggage（跨服务审计）
//   - 保留 *sql.DB 原生 API 风格（学习成本零）
//
// 不做的事：
//   - 不做 ORM / Query Builder
//   - 不做连接池监控（依赖底层 *sql.DB 自身的 DBStats）
//   - 不做 SQL 防注入（参数化查询由调用方保证）
//
// 用法：
//
//	import sqldriver "github.com/go-zeus/zeus/database/sql"
//	import _ "github.com/go-sql-driver/mysql"  // 注册驱动
//
//	db, err := sqldriver.New(database.DBOptions{
//	    Driver: "mysql",
//	    DSN:    "user:pass@tcp(127.0.0.1:3306)/db",
//	}, tracer, meter)
//	defer db.Close()
//
//	rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE age > ?", 18)
package sql

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"time"

	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/metrics"
	mnoop "github.com/go-zeus/zeus/metrics/noop"
	"github.com/go-zeus/zeus/trace"
	tnoop "github.com/go-zeus/zeus/trace/noop"
)

// 默认连接池参数（与 *sql.DB 零值不同，避免生产环境踩坑）
const (
	defaultMaxOpenConns    = 0 // 不限（与 stdlib 一致）
	defaultMaxIdleConns    = 2 // stdlib 默认 2
	defaultConnMaxLifetime = 0 // 永不重连（生产建议显式设置）
)

// 默认 metrics 名称（用户可覆盖）
const (
	metricQueryTotal    = "db_query_total"
	metricQueryDuration = "db_query_duration"
)

// db 包装 *sql.DB，注入 trace/metrics 钩子
type db struct {
	raw    *stdsql.DB
	tracer trace.Tracer
	meter  metrics.Meter
	name   string // 通常 = driver 名，用作 metric label
}

// Option 配置构造函数
type Option func(*db)

// WithTracer 注入 Tracer（默认 noop）
func WithTracer(t trace.Tracer) Option {
	return func(d *db) {
		if t != nil {
			d.tracer = t
		}
	}
}

// WithMeter 注入 Meter（默认 noop）
func WithMeter(m metrics.Meter) Option {
	return func(d *db) {
		if m != nil {
			d.meter = m
		}
	}
}

// WithName 设置 metric label 中的 db 标识（默认 driver 名）
//
// 多数据源场景下用业务名区分（如 "user-db" / "order-db"）
func WithName(name string) Option {
	return func(d *db) {
		if name != "" {
			d.name = name
		}
	}
}

// New 创建 DB 包装。
//
// 行为：
//   - sql.Open 不实际建连，Ping 时才验证 DSN
//   - 注入连接池默认参数（可被 opts.DBOptions 覆盖）
//   - tracer/meter 为 nil 时退化为 noop（不报错，便于轻量接入）
//   - 可变 opts 用于覆盖默认 name / tracer / meter
func New(opts database.DBOptions, t trace.Tracer, m metrics.Meter, additional ...Option) (database.DB, error) {
	if opts.Driver == "" {
		return nil, fmt.Errorf("database/sql: Driver is required")
	}
	raw, err := stdsql.Open(opts.Driver, opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("database/sql: open %s: %w", opts.Driver, err)
	}
	// 连接池参数：显式优于零值
	if opts.MaxOpenConns > 0 {
		raw.SetMaxOpenConns(opts.MaxOpenConns)
	} else {
		raw.SetMaxOpenConns(defaultMaxOpenConns)
	}
	if opts.MaxIdleConns > 0 {
		raw.SetMaxIdleConns(opts.MaxIdleConns)
	} else {
		raw.SetMaxIdleConns(defaultMaxIdleConns)
	}
	if opts.ConnMaxLifetime > 0 {
		raw.SetConnMaxLifetime(opts.ConnMaxLifetime)
	}

	d := &db{
		raw:    raw,
		tracer: t,
		meter:  m,
		name:   opts.Driver,
	}
	// 应用可变 Option（覆盖默认 name/tracer/meter）
	for _, opt := range additional {
		if opt != nil {
			opt(d)
		}
	}
	// 容错 nil tracer/meter（软依赖）
	if d.tracer == nil {
		d.tracer = tnoop.New()
	}
	if d.meter == nil {
		d.meter = mnoop.New()
	}
	return d, nil
}

// Query 执行返回多行的查询。
//
// 自动行为：
//   - EnsureTxID → ctx baggage 包含 zeus.tx.id
//   - 创建 span "db.query"（attrs: query, db, tx_id）
//   - 上报 metrics（counter + histogram）
func (d *db) Query(ctx context.Context, query string, args ...any) (database.Rows, error) {
	ctx, _ = database.EnsureTxID(ctx)
	spanCtx, span := d.startSpan(ctx, "db.query", query)
	start := time.Now()
	rows, err := d.raw.QueryContext(spanCtx, query, args...)
	d.recordMetric("query", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}
	span.End()
	return rows, nil
}

// QueryRow 执行返回单行的查询。
//
// 注意：*sql.Row 不暴露 Err，错误延迟到 Scan。span 在调用时即 End
// （无法等到 Scan），故无法记录 Scan 错误，这是 stdlib 设计限制。
func (d *db) QueryRow(ctx context.Context, query string, args ...any) database.Row {
	ctx, _ = database.EnsureTxID(ctx)
	spanCtx, span := d.startSpan(ctx, "db.query", query)
	start := time.Now()
	row := d.raw.QueryRowContext(spanCtx, query, args...)
	d.recordMetric("query", time.Since(start), nil)
	span.End()
	_ = ctx // ctx 用于 EnsureTxID 注入 baggage，QueryRowContext 用 spanCtx
	return row
}

// Exec 执行不返回行的语句（INSERT/UPDATE/DELETE/DDL）。
func (d *db) Exec(ctx context.Context, query string, args ...any) (stdsql.Result, error) {
	ctx, _ = database.EnsureTxID(ctx)
	spanCtx, span := d.startSpan(ctx, "db.exec", query)
	start := time.Now()
	res, err := d.raw.ExecContext(spanCtx, query, args...)
	d.recordMetric("exec", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
	}
	span.End()
	return res, err
}

// BeginTx 开启事务。
//
// 自动行为：
//   - EnsureTxID（同进程内所有 DB 操作共享 tx_id）
//   - 创建 span "db.tx.begin"
//   - 返回的 Tx 包装同样打点
func (d *db) BeginTx(ctx context.Context, opts ...database.TxOption) (database.Tx, error) {
	ctx, txID := database.EnsureTxID(ctx)

	sqlOpts := &stdsql.TxOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(sqlOpts)
		}
	}

	spanCtx, span := d.startSpan(ctx, "db.tx.begin", "BEGIN")
	start := time.Now()
	rawTx, err := d.raw.BeginTx(spanCtx, sqlOpts)
	d.recordMetric("tx_begin", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}
	span.End()
	return &tx{
		raw:    rawTx,
		txID:   txID,
		tracer: d.tracer,
		meter:  d.meter,
		name:   d.name,
	}, nil
}

// Ping 探活连接池（OnStart 时由 components 调用）
func (d *db) Ping(ctx context.Context) error {
	return d.raw.PingContext(ctx)
}

// Close 关闭连接池
func (d *db) Close() error {
	return d.raw.Close()
}

// tx 包装 *sql.Tx
type tx struct {
	raw    *stdsql.Tx
	txID   string
	tracer trace.Tracer
	meter  metrics.Meter
	name   string
}

func (t *tx) Query(ctx context.Context, query string, args ...any) (database.Rows, error) {
	// 事务内复用 txID（避免每个操作都生成新 ID）
	ctx = database.WithTxID(ctx, t.txID)
	spanCtx, span := t.startSpan(ctx, "db.tx.query", query)
	start := time.Now()
	rows, err := t.raw.QueryContext(spanCtx, query, args...)
	t.recordMetric("tx_query", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}
	span.End()
	return rows, nil
}

func (t *tx) QueryRow(ctx context.Context, query string, args ...any) database.Row {
	ctx = database.WithTxID(ctx, t.txID)
	spanCtx, span := t.startSpan(ctx, "db.tx.query", query)
	start := time.Now()
	row := t.raw.QueryRowContext(spanCtx, query, args...)
	t.recordMetric("tx_query", time.Since(start), nil)
	span.End()
	_ = ctx // ctx 用于注入 tx_id，QueryRowContext 用 spanCtx
	return row
}

func (t *tx) Exec(ctx context.Context, query string, args ...any) (stdsql.Result, error) {
	ctx = database.WithTxID(ctx, t.txID)
	spanCtx, span := t.startSpan(ctx, "db.tx.exec", query)
	start := time.Now()
	res, err := t.raw.ExecContext(spanCtx, query, args...)
	t.recordMetric("tx_exec", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
	}
	span.End()
	return res, err
}

func (t *tx) Commit() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = database.WithTxID(ctx, t.txID)
	// *sql.Tx.Commit 不接受 ctx，span 仍创建（记录提交耗时与错误）
	_, span := t.startSpan(ctx, "db.tx.commit", "COMMIT")
	start := time.Now()
	err := t.raw.Commit()
	t.recordMetric("tx_commit", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
	}
	span.End()
	return err
}

func (t *tx) Rollback() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = database.WithTxID(ctx, t.txID)
	_, span := t.startSpan(ctx, "db.tx.rollback", "ROLLBACK")
	start := time.Now()
	err := t.raw.Rollback()
	t.recordMetric("tx_rollback", time.Since(start), err)
	if err != nil {
		span.RecordError(err)
	}
	span.End()
	return err
}

// startSpan 创建带标准 attrs 的 span
//
// attrs: db（连接名）、query（SQL）、tx_id（事务 ID）
func (d *db) startSpan(ctx context.Context, name, query string) (context.Context, trace.Span) {
	txID := database.TxIDFromContext(ctx)
	attrs := map[string]string{
		"db":    d.name,
		"query": query,
	}
	if txID != "" {
		attrs["tx_id"] = txID
	}
	return d.tracer.StartSpan(ctx, name, withAttrs(attrs))
}

func (t *tx) startSpan(ctx context.Context, name, query string) (context.Context, trace.Span) {
	attrs := map[string]string{
		"db":    t.name,
		"query": query,
		"tx_id": t.txID,
	}
	return t.tracer.StartSpan(ctx, name, withAttrs(attrs))
}

// withAttrs 便捷构造 SpanOption（trace 包未导出 WithAttributes，本地实现）
func withAttrs(attrs map[string]string) trace.SpanOption {
	return func(cfg *trace.SpanConfig) {
		cfg.Attrs = attrs
	}
}

// recordMetric 上报 query 计数 + 延迟
//
// status: "ok" / "error"
func (d *db) recordMetric(op string, dur time.Duration, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	labels := map[string]string{"db": d.name, "op": op, "status": status}
	d.meter.Counter(metricQueryTotal, labels).Inc()
	d.meter.Histogram(metricQueryDuration, map[string]string{"db": d.name, "op": op}).Observe(dur.Seconds())
}

func (t *tx) recordMetric(op string, dur time.Duration, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	labels := map[string]string{"db": t.name, "op": op, "status": status}
	t.meter.Counter(metricQueryTotal, labels).Inc()
	t.meter.Histogram(metricQueryDuration, map[string]string{"db": t.name, "op": op}).Observe(dur.Seconds())
}
