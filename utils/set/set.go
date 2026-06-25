// Package set 提供基于泛型的集合类型 Set[T]。
//
// 设计目的：
//   - 标准库无 Set 类型（map[T]struct{} 是惯用法但缺操作）
//   - 包装 map[T]struct{} 提供常用集合代数（并/交/差/子集）
//   - 零依赖（仅标准库）
//
// 与 slices 的关系：
//   - slices 保留顺序 + 重复
//   - set 不保留顺序 + 自动去重
//   - 二者可互转：FromSlice / Values
//
// 并发安全：
//   - Set 本身非线程安全；并发使用请加锁或使用 sync.Set 包装
//   - 所有方法均非 reentrant
package set

import "sort"

// Set 是基于 map 的泛型集合
type Set[T comparable] struct {
	m map[T]struct{}
}

// New 创建空 Set
func New[T comparable]() Set[T] {
	return Set[T]{m: make(map[T]struct{})}
}

// NewWithCapacity 创建指定容量的 Set（仅做容量预估，不影响语义）
//
// 用途：预先知道元素数量时减少 rehash
func NewWithCapacity[T comparable](n int) Set[T] {
	return Set[T]{m: make(map[T]struct{}, n)}
}

// FromSlice 从切片创建 Set（自动去重）
//
// 示例：
//
//	s := set.FromSlice([]int{1, 2, 2, 3}) // {1, 2, 3}
func FromSlice[T comparable](items []T) Set[T] {
	s := NewWithCapacity[T](len(items))
	for _, v := range items {
		s.Add(v)
	}
	return s
}

// Add 添加元素；若已存在返回 false（语义类似 map 赋值）
func (s Set[T]) Add(v T) bool {
	if _, ok := s.m[v]; ok {
		return false
	}
	s.m[v] = struct{}{}
	return true
}

// Remove 删除元素；若不存在返回 false
func (s Set[T]) Remove(v T) bool {
	if _, ok := s.m[v]; !ok {
		return false
	}
	delete(s.m, v)
	return true
}

// Contains 判断 v 是否在 Set 中
func (s Set[T]) Contains(v T) bool {
	_, ok := s.m[v]
	return ok
}

// Has 是 Contains 的别名（部分场景下 Has 读起来更自然）
func (s Set[T]) Has(v T) bool {
	return s.Contains(v)
}

// Size 返回元素个数
func (s Set[T]) Size() int {
	return len(s.m)
}

// IsEmpty 判断 Set 是否为空
func (s Set[T]) IsEmpty() bool {
	return len(s.m) == 0
}

// Clear 清空 Set
func (s Set[T]) Clear() {
	for k := range s.m {
		delete(s.m, k)
	}
}

// Values 返回所有元素切片（顺序未定义）
//
// 若需稳定顺序，用 SortedValues
func (s Set[T]) Values() []T {
	out := make([]T, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	return out
}

// SortedValues 返回排序后的元素切片（要求 T 实现 sort.Interface）
//
// 仅对原生有序类型（int/string/float...）有效，自定义类型需传 less 函数
func SortedValues[T comparable](s Set[T], less func(a, b T) bool) []T {
	out := s.Values()
	sort.Slice(out, func(i, j int) bool {
		return less(out[i], out[j])
	})
	return out
}

// Range 遍历 Set（顺序未定义）；fn 返回 false 提前终止
func (s Set[T]) Range(fn func(v T) bool) {
	for k := range s.m {
		if !fn(k) {
			return
		}
	}
}

// Clone 返回 Set 的副本（深拷贝）
func (s Set[T]) Clone() Set[T] {
	out := NewWithCapacity[T](len(s.m))
	for k := range s.m {
		out.m[k] = struct{}{}
	}
	return out
}

// Union 返回并集（s ∪ other），不修改 s 与 other
func (s Set[T]) Union(other Set[T]) Set[T] {
	out := NewWithCapacity[T](s.Size() + other.Size())
	for k := range s.m {
		out.m[k] = struct{}{}
	}
	for k := range other.m {
		out.m[k] = struct{}{}
	}
	return out
}

// Intersect 返回交集（s ∩ other）
func (s Set[T]) Intersect(other Set[T]) Set[T] {
	out := New[T]()
	// 遍历较小集合以减少迭代次数
	small, big := &s, &other
	if small.Size() > big.Size() {
		small, big = big, small
	}
	for k := range small.m {
		if _, ok := big.m[k]; ok {
			out.m[k] = struct{}{}
		}
	}
	return out
}

// Difference 返回差集（s - other，即 s 有但 other 没有的元素）
func (s Set[T]) Difference(other Set[T]) Set[T] {
	out := NewWithCapacity[T](s.Size())
	for k := range s.m {
		if _, ok := other.m[k]; !ok {
			out.m[k] = struct{}{}
		}
	}
	return out
}

// SymmetricDifference 返回对称差集（s △ other，即仅在其中一个集合的元素）
func (s Set[T]) SymmetricDifference(other Set[T]) Set[T] {
	out := New[T]()
	for k := range s.m {
		if _, ok := other.m[k]; !ok {
			out.m[k] = struct{}{}
		}
	}
	for k := range other.m {
		if _, ok := s.m[k]; !ok {
			out.m[k] = struct{}{}
		}
	}
	return out
}

// IsSubset 判断 s 是否是 other 的子集（s ⊆ other）
func (s Set[T]) IsSubset(other Set[T]) bool {
	if s.Size() > other.Size() {
		return false
	}
	for k := range s.m {
		if _, ok := other.m[k]; !ok {
			return false
		}
	}
	return true
}

// IsSuperset 判断 s 是否是 other 的超集（s ⊇ other）
func (s Set[T]) IsSuperset(other Set[T]) bool {
	return other.IsSubset(s)
}

// IsDisjoint 判断 s 与 other 是否不相交（无共同元素）
func (s Set[T]) IsDisjoint(other Set[T]) bool {
	small, big := &s, &other
	if small.Size() > big.Size() {
		small, big = big, small
	}
	for k := range small.m {
		if _, ok := big.m[k]; ok {
			return false
		}
	}
	return true
}

// Equal 判断 s 与 other 是否包含相同元素
func (s Set[T]) Equal(other Set[T]) bool {
	if s.Size() != other.Size() {
		return false
	}
	for k := range s.m {
		if _, ok := other.m[k]; !ok {
			return false
		}
	}
	return true
}
