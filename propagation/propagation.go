// Package propagation 提供跨进程边界的通用 K-V 上下文传播。
//
// 设计目的：
//   - 替代 routing 包硬编码单一 X-Zeus-Cluster 的局限
//   - 支持用户自定义任意 K-V（如 tenant.id / feature.flag / user.tier）的全链路传播
//   - 对齐 W3C Baggage 规范，跨语言互操作（Java/Python/Node 等服务也能识别）
//
// 核心概念：
//   - Entry：单个 K-V 对（Key + Value）
//   - Bag：Entry 集合（顺序保留，去重 by Key）
//   - 跨进程格式：HTTP Header `Baggage: k1=v1,k2=v2`（W3C 标准）
//
// 与 routing 包的关系：
//   - routing 是 propagation 的特化（cluster=xxx 这一个 K-V）
//   - routing.WithCluster 等价于 propagation.With(ctx, "zeus.cluster", "canary")
//   - 新代码推荐直接用 propagation，routing 保持向后兼容
//
// 使用示例：
//
//	// 注入业务 K-V
//	ctx = propagation.With(ctx, "tenant.id", "acme")
//	ctx = propagation.With(ctx, "feature.flag", "beta")
//
//	// 通过 zeus client 调下游（自动 InjectHTTP）
//	client.Do(ctx, req)
//
//	// 下游服务入口（自动 ExtractHTTP → ctx）
//	val := propagation.Get(ctx, "tenant.id")  // "acme"
package propagation

import "context"

// HeaderBaggage W3C Baggage HTTP Header 名称（大小写不敏感，按 W3C 规范用首字母大写形式）
const HeaderBaggage = "Baggage"

// MetadataBaggage gRPC metadata key（小写，符合 gRPC 规范）
const MetadataBaggage = "baggage"

// ctxKey context 中 Bag 的 key 类型（不导出）
type ctxKey struct{}

// Entry 单个 K-V 对
//
// Key 必须是合法的 token（字母数字 + 部分 symbol，详见 isTokenChar）。
// Value 可以是任意 UTF-8 字符串，inject 时自动 percent-encode 非 token 字符。
type Entry struct {
	Key   string
	Value string
}

// Bag K-V 集合，按插入顺序保留 Entry；同一 Key 后写覆盖前写。
//
// 不可变结构：所有修改操作返回新 Bag。
// 设计动机：context.Value 共享同一指针，不可变避免并发问题。
type Bag struct {
	entries []Entry
	index   map[string]int // key → entries 索引，去重用
}

// NewBag 创建空 Bag
func NewBag() *Bag {
	return &Bag{index: make(map[string]int)}
}

// With 返回追加了新 K-V 的 Bag（若 Key 已存在则覆盖）。
// key 为空时返回原 Bag。
func (b *Bag) With(key, value string) *Bag {
	if key == "" {
		return b
	}
	if b == nil {
		b = NewBag()
	}
	next := b.clone()
	if idx, ok := next.index[key]; ok {
		next.entries[idx] = Entry{Key: key, Value: value}
		return next
	}
	next.index[key] = len(next.entries)
	next.entries = append(next.entries, Entry{Key: key, Value: value})
	return next
}

// WithEntries 批量追加（按顺序，同 Key 后写覆盖）
func (b *Bag) WithEntries(entries ...Entry) *Bag {
	next := b
	for _, e := range entries {
		next = next.With(e.Key, e.Value)
	}
	return next
}

// Get 按 Key 查询值；不存在返回 ("", false)
func (b *Bag) Get(key string) (string, bool) {
	if b == nil || key == "" {
		return "", false
	}
	if idx, ok := b.index[key]; ok {
		return b.entries[idx].Value, true
	}
	return "", false
}

// Entries 返回所有 Entry 的副本（调用方可任意修改）
func (b *Bag) Entries() []Entry {
	if b == nil || len(b.entries) == 0 {
		return nil
	}
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Len 返回 Entry 数量
func (b *Bag) Len() int {
	if b == nil {
		return 0
	}
	return len(b.entries)
}

// clone 复制一份可修改的 Bag（共享 index 时仍需重建）
func (b *Bag) clone() *Bag {
	if b == nil {
		return NewBag()
	}
	next := &Bag{
		entries: make([]Entry, len(b.entries)),
		index:   make(map[string]int, len(b.index)),
	}
	copy(next.entries, b.entries)
	for k, v := range b.index {
		next.index[k] = v
	}
	return next
}

// With 向 ctx 注入 K-V。
//
// 多次调用累积：propagation.With(propagation.With(ctx, "a", "1"), "b", "2") → Bag{a=1, b=2}
// 已存在的 Key 会被覆盖。
//
// 返回新 ctx，原 ctx 不变（context.Value 语义）。
func With(ctx context.Context, key, value string) context.Context {
	if key == "" {
		return ctx
	}
	existing, _ := ctx.Value(ctxKey{}).(*Bag)
	next := existing.With(key, value)
	return context.WithValue(ctx, ctxKey{}, next)
}

// WithEntries 批量注入
func WithEntries(ctx context.Context, entries ...Entry) context.Context {
	if len(entries) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(ctxKey{}).(*Bag)
	next := existing.WithEntries(entries...)
	return context.WithValue(ctx, ctxKey{}, next)
}

// FromContext 读取 ctx 内的 Bag；缺失返回 nil。
//
// 注意：返回的是内部 Bag 的只读视图，调用方不应修改（虽然修改不会影响 ctx，但避免误用）。
// 如需修改请用 propagation.With / WithEntries 重新派生 ctx。
func FromContext(ctx context.Context) *Bag {
	if ctx == nil {
		return nil
	}
	bag, _ := ctx.Value(ctxKey{}).(*Bag)
	return bag
}

// Get 从 ctx 按 Key 读取值；不存在返回 ("", false)
func Get(ctx context.Context, key string) (string, bool) {
	return FromContext(ctx).Get(key)
}
