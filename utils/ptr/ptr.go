// Package ptr 提供指针相关泛型辅助函数。
//
// 设计目的：
//   - 简化 *T 字面量构造（避免在调用方声明临时变量）
//   - 安全解引用 *T（带默认值兜底）
//   - 泛型 *T 比较（== / DeepEqual 两种语义）
//
// 典型场景：
//
//	addr := ptr.Of(42)              // *int 指向 42
//	v := ptr.Deref(addr, 0)        // 解引用，nil 返回默认 0
//	eq := ptr.Equal(addr, addr2)   // 指针值相等（含 nil == nil）
//
// 与标准库的关系：
//   - Go 1.22 标准库尚无泛型指针工具（Go 1.23 才有 cmp.Or）
//   - 与 slicesx 互补，共同构成"基础工具箱"
package ptr

import "reflect"

// Of 把值 v 装箱为 *T。
//
// 等价于：
//
//	v := 42
//	p := &v
//
// 但适合在表达式上下文使用（如函数参数）：
//
//	cfg := Config{Timeout: ptr.Of(10 * time.Second)}
func Of[T any](v T) *T {
	return &v
}

// Deref 解引用 p；若 p == nil 返回 def。
//
// 行为：避免调用方写 `if p != nil { v = *p } else { v = def }`
//
// 示例：
//
//	v := ptr.Deref(cfg.Timeout, time.Minute) // cfg.Timeout == nil 时返回 time.Minute
func Deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// Equal 比较两个 *T 是否指向相同的值（或同为 nil）。
//
// 行为：
//   - 两者都 nil → true
//   - 仅一个 nil → false
//   - 都非 nil → 比较 *a == *b（用 == 比较，要求 T 是 comparable）
//
// 适用：comparable 类型（int/string/struct{...} 含 comparable 字段）
//
// 示例：
//
//	if ptr.Equal(cfg.Timeout, other.Timeout) { ... }
func Equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b // 两者都 nil → true；否则 false
	}
	return *a == *b
}

// DeepEqual 用 reflect.DeepEqual 比较 *a 与 *b 的值。
//
// 适用：T 不可比较（如含 slice / map / func 字段）
//
// 性能：比 Equal 慢 10 倍以上，仅在类型不 comparable 时使用
func DeepEqual[T any](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(*a, *b)
}

// IsNil 判断 p 是否为 nil。
//
// 提供 IsNil 而非 ptr == nil 直接比较，是为了：
//   - API 风格统一（包内其他工具都按函数调用）
//   - 当 *T 来自接口字段时（interface 内 *T），简化类型断言流程
func IsNil[T any](p *T) bool {
	return p == nil
}

// Clone 复制 p 指向的值，返回新指针；p == nil 返回 nil。
//
// 用途：避免外部修改影响原值
//
// 示例：
//
//	cfg := ptr.Clone(baseConfig) // 修改 *cfg 不影响 *baseConfig
func Clone[T any](p *T) *T {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}
