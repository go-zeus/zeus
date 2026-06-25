// Package database 提供数据库访问的统一抽象。
//
// 设计动机：
//   - 让 DB 操作像 HTTP handler 一样获得 trace / metrics / propagation 自动集成
//   - 主包零依赖（薄封装 stdlib database/sql，不引入 ORM）
//   - 第三方实现（MySQL/Postgres/Redis）放 plugins/database/<vendor>
//
// 抽象边界：
//   - 暴露 DB / Tx / Rows / Row 接口，签名几乎对齐 *sql.DB（学习成本零）
//   - 每个 Query / Exec 自动：
//     1) 注入 tx_id 到 ctx baggage（缺失则生成 UUID）
//     2) 创建 trace span（attrs: query, db, tx_id）
//     3) 上报 metrics（counter + histogram）
//   - 不做 ORM / Query Builder / 关联查询（避免与 sqlx/gorm/ent 竞争）
//
// 事务上下文传播：
//   - 同进程：WithTx(ctx, tx) → FromTx(ctx) 让多个 Repository 共享同一 Tx
//   - 跨服务：tx_id 通过 propagation.Bag 自动透传（用于审计/排查，不做 2PC）
//
// 适用场景：
//   - 微服务后端的 Repository 层基础设施
//   - 单元测试的 mock DB（通过实现 DB 接口）
//
// 不适用：
//   - 需要复杂 ORM 能力（用 gorm/ent）
//   - 需要跨服务分布式事务（用 Seata / TCC / Saga）
package database

import (
	"context"
	stdsql "database/sql"
	"errors"
	"time"

	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/utils/uuid"
)

// ErrNoTx 当前 ctx 内未找到事务时返回。
//
// 触发场景：调用 FromTx(ctx) 但 ctx 未通过 WithTx 注入事务句柄
// （通常是业务忘记开启事务或跨 goroutine 传递 ctx 时丢失）。
// 处理建议：调用方应判断 errors.Is(err, ErrNoTx) 后要么开启新事务，
// 要么降级为非事务路径，不要直接 panic。
var ErrNoTx = errors.New("database: no tx in context")

// DB 数据库连接抽象（薄封装 *sql.DB + trace/metrics/propagation 钩子）。
//
// 与 *sql.DB 的差异：
//   - 所有方法首参为 ctx（强制传播，便于 trace/tx_id 注入）
//   - Query/Exec 自动注入 tx_id 到 ctx baggage
//   - 自动创建 trace span + 上报 metrics
//
// 不直接暴露 *sql.DB：避免用户绕过 hook；如需原生句柄由实现包按需提供。
type DB interface {
	// Query 执行返回多行的查询
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	// QueryRow 执行返回单行的查询（错误延迟到 Scan 时返回）
	QueryRow(ctx context.Context, query string, args ...any) Row
	// Exec 执行不返回行的语句（INSERT/UPDATE/DELETE/DDL）
	Exec(ctx context.Context, query string, args ...any) (stdsql.Result, error)
	// BeginTx 开启事务；opts 可指定隔离级别/只读等
	BeginTx(ctx context.Context, opts ...TxOption) (Tx, error)
	// Ping 探活（OnStart 时由 components 调用验证连接）
	Ping(ctx context.Context) error
	// Close 关闭连接池
	Close() error
}

// Tx 事务抽象。
//
// 行为契约：
//   - Commit 前任何错误应调用 Rollback
//   - Commit/Rollback 后再调用方法返回 sql.ErrTxDone
type Tx interface {
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) Row
	Exec(ctx context.Context, query string, args ...any) (stdsql.Result, error)
	Commit() error
	Rollback() error
}

// Rows 多行结果集（薄封装 *sql.Rows）。
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Row 单行结果（薄封装 *sql.Row）。
//
// 错误通过 Scan 返回；调用 Err() 可提前探测。
type Row interface {
	Scan(dest ...any) error
	Err() error
}

// DBOptions 连接池配置。
//
// 字段含义与 stdlib sql.DB 一致，零值时由实现按合理默认填充。
type DBOptions struct {
	// Driver 驱动名（如 "mysql" / "postgres" / "sqlite3"）
	Driver string
	// DSN 数据源名称
	DSN string
	// MaxOpenConns 最大打开连接数（0 = 不限）
	MaxOpenConns int
	// MaxIdleConns 最大空闲连接数
	MaxIdleConns int
	// ConnMaxLifetime 连接最长存活时间
	ConnMaxLifetime time.Duration
}

// TxOption 事务选项。
type TxOption func(*stdsql.TxOptions)

// WithIsolation 设置事务隔离级别
func WithIsolation(level stdsql.IsolationLevel) TxOption {
	return func(o *stdsql.TxOptions) { o.Isolation = level }
}

// WithReadOnly 设置只读事务（驱动可据此优化）
func WithReadOnly(ro bool) TxOption {
	return func(o *stdsql.TxOptions) { o.ReadOnly = ro }
}

// —— 同进程事务传播 ——
//
// ctx 内传递 Tx 句柄，让多个 Repository 共享同一事务：
//
//	tx, _ := db.BeginTx(ctx)
//	ctx = database.WithTx(ctx, tx)
//	repoA.Update(ctx, ...)
//	repoB.Insert(ctx, ...)
//	tx.Commit()
//
// 实现：Repository 内部 FromTx(ctx) 优先取事务句柄，回退到全局 DB。

type txKey struct{}

// WithTx 把事务句柄放入 ctx
func WithTx(ctx context.Context, tx Tx) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, txKey{}, tx)
}

// FromTx 从 ctx 取事务句柄；不存在返回 (nil, false)
func FromTx(ctx context.Context) (Tx, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(txKey{}).(Tx)
	return tx, ok
}

// —— 跨服务 tx_id 传播 ——
//
// tx_id 是事务的"业务标识"，通过 propagation.Bag 自动随 client 透传到下游：
//   - 同进程：所有 DB 操作共享同一 tx_id（BeginTx 时生成，Commit 后失效）
//   - 跨服务：下游服务的 DB 操作也带上同一 tx_id，便于全链路审计
//
// 设计权衡：
//   - 不实现 2PC / Saga / TCC（业务侧自行选型）
//   - tx_id 仅用于可观测性关联（trace span attribute / log field / metrics label）

// TxIDKey propagation baggage 中的 tx_id key（用户可见，可自定义覆盖）
const TxIDKey = "zeus.tx.id"

type txIDCtxKey struct{}

// WithTxID 把 tx_id 写入 ctx（同时同步到 propagation baggage 便于跨服务透传）。
//
// 行为：
//   - 覆盖已有 tx_id
//   - 空 id 视为 no-op
func WithTxID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if id == "" {
		return ctx
	}
	ctx = context.WithValue(ctx, txIDCtxKey{}, id)
	// 同步写入 baggage，让 client/server 自动透传到下游
	return propagation.With(ctx, TxIDKey, id)
}

// TxIDFromContext 读取 ctx 内的 tx_id；不存在返回 ""
func TxIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	// 优先读 ctx 本地值（同进程内最权威）
	if v, ok := ctx.Value(txIDCtxKey{}).(string); ok && v != "" {
		return v
	}
	// 兜底从 baggage 读（跨服务入站场景）
	if v, ok := propagation.Get(ctx, TxIDKey); ok {
		return v
	}
	return ""
}

// EnsureTxID 确保 ctx 内有 tx_id；缺失则生成 UUID 并写入。
//
// 返回值：更新后的 ctx + 当前 tx_id
//
// 用法：DB 实现的 Query/Exec 入口调用，保证每次操作都能关联到 tx_id。
func EnsureTxID(ctx context.Context) (context.Context, string) {
	if id := TxIDFromContext(ctx); id != "" {
		return ctx, id
	}
	id := uuid.New()
	return WithTxID(ctx, id), id
}
