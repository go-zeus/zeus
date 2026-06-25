package database

import (
	"context"
	stdsql "database/sql"
	"testing"

	"github.com/go-zeus/zeus/propagation"
)

// fakeTx 仅用于 ctx 传播测试（不实现真实 DB 行为）
type fakeTx struct{}

func (fakeTx) Query(context.Context, string, ...any) (Rows, error)         { return nil, nil }
func (fakeTx) QueryRow(context.Context, string, ...any) Row                { return nil }
func (fakeTx) Exec(context.Context, string, ...any) (stdsql.Result, error) { return nil, nil }
func (fakeTx) Commit() error                                               { return nil }
func (fakeTx) Rollback() error                                             { return nil }

// TestWithTx_FromTx 同进程事务句柄 ctx 传播
func TestWithTx_FromTx(t *testing.T) {
	tx := fakeTx{}
	ctx := WithTx(context.Background(), tx)

	got, ok := FromTx(ctx)
	if !ok {
		t.Fatal("FromTx: expected ok=true")
	}
	if got != tx {
		t.Errorf("FromTx: got %v, want %v", got, tx)
	}
}

// TestFromTx_Empty ctx 内无事务返回 (nil, false)
func TestFromTx_Empty(t *testing.T) {
	if tx, ok := FromTx(context.Background()); ok || tx != nil {
		t.Errorf("FromTx on empty ctx = (%v,%v), want (nil,false)", tx, ok)
	}
	if tx, ok := FromTx(context.TODO()); ok || tx != nil {
		t.Errorf("FromTx on nil ctx = (%v,%v), want (nil,false)", tx, ok)
	}
}

// TestWithTxID_TxIDFromContext tx_id 写入与读取
func TestWithTxID_TxIDFromContext(t *testing.T) {
	ctx := WithTxID(context.Background(), "tx-abc")
	if got := TxIDFromContext(ctx); got != "tx-abc" {
		t.Errorf("TxIDFromContext = %q, want tx-abc", got)
	}
}

// TestWithTxID_EmptyID 空 id 视为 no-op
func TestWithTxID_EmptyID(t *testing.T) {
	ctx := WithTxID(context.Background(), "")
	if got := TxIDFromContext(ctx); got != "" {
		t.Errorf("TxIDFromContext after empty WithTxID = %q, want empty", got)
	}
}

// TestWithTxID_PropagationBaggage tx_id 自动同步到 propagation baggage
//
// 关键契约：tx_id 写入后，下游服务通过 propagation.Get 能读到
func TestWithTxID_PropagationBaggage(t *testing.T) {
	ctx := WithTxID(context.Background(), "tx-xyz")
	v, ok := propagation.Get(ctx, TxIDKey)
	if !ok || v != "tx-xyz" {
		t.Errorf("propagation.Get(%q) = (%q,%v), want (tx-xyz,true)", TxIDKey, v, ok)
	}
}

// TestTxIDFromContext_FromBaggage 入站 baggage 内的 tx_id 可被读出
//
// 场景：上游服务通过 HTTP/gRPC 把 tx_id 透传到本服务，本服务 ctx 内能读到
func TestTxIDFromContext_FromBaggage(t *testing.T) {
	// 模拟上游透传：把 tx_id 注入 baggage（不经 WithTxID，直接用 propagation）
	ctx := propagation.With(context.Background(), TxIDKey, "tx-from-upstream")
	if got := TxIDFromContext(ctx); got != "tx-from-upstream" {
		t.Errorf("TxIDFromContext from baggage = %q, want tx-from-upstream", got)
	}
}

// TestTxIDFromContext_PreferCtxOverBaggage ctx 本地值优先于 baggage
func TestTxIDFromContext_PreferCtxOverBaggage(t *testing.T) {
	// 先在 baggage 写 tx-baggage
	ctx := propagation.With(context.Background(), TxIDKey, "tx-baggage")
	// 再用 WithTxID 覆盖为 tx-local（同时同步 baggage）
	ctx = WithTxID(ctx, "tx-local")
	if got := TxIDFromContext(ctx); got != "tx-local" {
		t.Errorf("TxIDFromContext = %q, want tx-local (ctx local wins)", got)
	}
}

// TestEnsureTxID_Missing 缺失 tx_id 时自动生成
func TestEnsureTxID_Missing(t *testing.T) {
	ctx, id := EnsureTxID(context.Background())
	if id == "" {
		t.Error("EnsureTxID: generated id is empty")
	}
	// 生成的 id 必须可被 TxIDFromContext 读出
	if got := TxIDFromContext(ctx); got != id {
		t.Errorf("TxIDFromContext after EnsureTxID = %q, want %q", got, id)
	}
	// 同时必须同步到 baggage
	if v, ok := propagation.Get(ctx, TxIDKey); !ok || v != id {
		t.Errorf("propagation.Get(%q) = (%q,%v), want (%q,true)", TxIDKey, v, ok, id)
	}
}

// TestEnsureTxID_Existing 已有 tx_id 时不重新生成
func TestEnsureTxID_Existing(t *testing.T) {
	orig := WithTxID(context.Background(), "preserve-me")
	ctx, id := EnsureTxID(orig)
	if id != "preserve-me" {
		t.Errorf("EnsureTxID overwrote existing id: got %q, want preserve-me", id)
	}
	if ctx != orig {
		// ctx 应该未被修改（已有 id 直接返回原 ctx）
		t.Error("EnsureTxID should return original ctx when id already present")
	}
}

// TestEnsureTxID_NilContext nil ctx 不 panic
func TestEnsureTxID_NilContext(t *testing.T) {
	ctx, id := EnsureTxID(context.TODO())
	if id == "" {
		t.Error("EnsureTxID(nil): id should not be empty")
	}
	if ctx == nil {
		t.Error("EnsureTxID(nil): ctx should not be nil")
	}
}

// TestTxOption_WithIsolation 隔离级别选项
func TestTxOption_WithIsolation(t *testing.T) {
	opts := &stdsql.TxOptions{}
	WithIsolation(stdsql.LevelSerializable)(opts)
	if opts.Isolation != stdsql.LevelSerializable {
		t.Errorf("Isolation = %v, want LevelSerializable", opts.Isolation)
	}
}

// TestTxOption_WithReadOnly 只读选项
func TestTxOption_WithReadOnly(t *testing.T) {
	opts := &stdsql.TxOptions{}
	WithReadOnly(true)(opts)
	if !opts.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
}

// TestErrNoTx 错误变量存在
func TestErrNoTx(t *testing.T) {
	if ErrNoTx == nil {
		t.Error("ErrNoTx should not be nil")
	}
	if ErrNoTx.Error() == "" {
		t.Error("ErrNoTx should have non-empty message")
	}
}
