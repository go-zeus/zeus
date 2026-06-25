// Package slicesx 提供基于 Go 1.22+ 泛型的切片辅助函数。
//
// 设计目的：
//   - 补标准库 slices 缺失的高频操作（Map / Filter / Reduce / GroupBy / Unique / Find）
//   - 所有函数纯函数式（不修改入参），保证并发安全
//   - 零依赖（仅标准库）
//
// 与标准库 slices 的关系：
//   - 标准库 slices 提供 in-place 操作（Reverse / Sort / Delete）
//   - slicesx 提供 transform 操作（产生新 slice）
//   - 二者互补，不重复实现
//
// 命名约定：
//   - 函数名首字母大写 + 短名（Map/Filter/Reduce），便于链式调用
//   - 谓词参数统一命名 pred / ok，避免歧义
package slicesx

import "slices"

// Map 把 []T 映射为 []U（保留顺序 + 长度）
//
// 示例：
//
//	nums := []int{1, 2, 3}
//	doubled := slicesx.Map(nums, func(n int) int { return n * 2 }) // [2, 4, 6]
func Map[T, U any](in []T, f func(T) U) []U {
	if in == nil {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// Filter 保留满足 pred 的元素（保留顺序）
//
// 示例：
//
//	nums := []int{1, 2, 3, 4}
//	evens := slicesx.Filter(nums, func(n int) bool { return n%2 == 0 }) // [2, 4]
func Filter[T any](in []T, pred func(T) bool) []T {
	if in == nil {
		return nil
	}
	out := make([]T, 0, len(in))
	for _, v := range in {
		if pred(v) {
			out = append(out, v)
		}
	}
	return out
}

// Reduce 归约为单个值
//
// 示例：
//
//	nums := []int{1, 2, 3, 4}
//	sum := slicesx.Reduce(nums, 0, func(acc, n int) int { return acc + n }) // 10
func Reduce[T, U any](in []T, init U, f func(U, T) U) U {
	acc := init
	for _, v := range in {
		acc = f(acc, v)
	}
	return acc
}

// Contains 切片中是否包含 v（基于 == 比较）
//
// 等价于标准库 slices.Contains，提供此别名保持包内一致命名
func Contains[T comparable](in []T, v T) bool {
	return slices.Contains(in, v)
}

// ContainsFunc 切片中是否存在满足 pred 的元素
//
// 等价于标准库 slices.ContainsFunc，提供此别名保持包内一致命名
func ContainsFunc[T any](in []T, pred func(T) bool) bool {
	return slices.ContainsFunc(in, pred)
}

// Find 返回第一个满足 pred 的元素 + 其索引；未找到返回零值 + -1
//
// 示例：
//
//	users := []User{{ID: 1, Name: "alice"}, {ID: 2, Name: "bob"}}
//	u, idx := slicesx.Find(users, func(u User) bool { return u.ID == 2 })
//	// u = User{2, "bob"}, idx = 1
func Find[T any](in []T, pred func(T) bool) (T, int) {
	for i, v := range in {
		if pred(v) {
			return v, i
		}
	}
	var zero T
	return zero, -1
}

// FindLast 返回最后一个满足 pred 的元素 + 其索引；未找到返回零值 + -1
func FindLast[T any](in []T, pred func(T) bool) (T, int) {
	for i := len(in) - 1; i >= 0; i-- {
		if pred(in[i]) {
			return in[i], i
		}
	}
	var zero T
	return zero, -1
}

// Unique 去重（保留首次出现顺序，基于 == 比较）
//
// 示例：
//
//	nums := []int{1, 2, 2, 3, 1, 4}
//	uniq := slicesx.Unique(nums) // [1, 2, 3, 4]
//
// 复杂度：O(n) 时间 + O(n) 空间（用 map 去重）
func Unique[T comparable](in []T) []T {
	if in == nil {
		return nil
	}
	seen := make(map[T]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// UniqueBy 用 key 提取函数去重（保留首次出现顺序）
//
// 示例：
//
//	users := []User{{ID: 1}, {ID: 2}, {ID: 1}}
//	uniq := slicesx.UniqueBy(users, func(u User) int { return u.ID }) // [{1}, {2}]
func UniqueBy[T any, K comparable](in []T, key func(T) K) []T {
	if in == nil {
		return nil
	}
	seen := make(map[K]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		k := key(v)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	return out
}

// GroupBy 按 key 分组
//
// 示例：
//
//	users := []User{{Team: "A", Name: "alice"}, {Team: "B", Name: "bob"}, {Team: "A", Name: "carol"}}
//	byTeam := slicesx.GroupBy(users, func(u User) string { return u.Team })
//	// map[string][]User{"A": [{alice}, {carol}], "B": [{bob}]}
func GroupBy[T any, K comparable](in []T, key func(T) K) map[K][]T {
	if in == nil {
		return make(map[K][]T)
	}
	out := make(map[K][]T, len(in))
	for _, v := range in {
		k := key(v)
		out[k] = append(out[k], v)
	}
	return out
}

// Chunk 把切片分成大小为 n 的子切片
//
// 示例：
//
//	nums := []int{1, 2, 3, 4, 5}
//	chunks := slicesx.Chunk(nums, 2) // [[1, 2], [3, 4], [5]]
//
// 行为：
//   - n <= 0 返回 nil（无效输入）
//   - 最后一个 chunk 可能小于 n
//   - 入参 nil 返回 nil
func Chunk[T any](in []T, n int) [][]T {
	if in == nil {
		return nil
	}
	if n <= 0 {
		return nil
	}
	out := make([][]T, 0, (len(in)+n-1)/n)
	for i := 0; i < len(in); i += n {
		end := i + n
		if end > len(in) {
			end = len(in)
		}
		// 这里 copy 而非 reslice：避免外部修改 chunk 影响原切片
		chunk := make([]T, end-i)
		copy(chunk, in[i:end])
		out = append(out, chunk)
	}
	return out
}

// Flat 把二维切片展平
//
// 示例：
//
//	nested := [][]int{{1, 2}, {3, 4}, {5}}
//	flat := slicesx.Flat(nested) // [1, 2, 3, 4, 5]
func Flat[T any](in [][]T) []T {
	total := 0
	for _, sub := range in {
		total += len(sub)
	}
	out := make([]T, 0, total)
	for _, sub := range in {
		out = append(out, sub...)
	}
	return out
}

// Reverse 返回反转后的新切片（不修改入参）
//
// 等价于 slices.Reverse + copy，但纯函数式
func Reverse[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	for i, v := range in {
		out[len(in)-1-i] = v
	}
	return out
}

// AnyMatch 任意元素满足 pred 返回 true（空切片返回 false）
func AnyMatch[T any](in []T, pred func(T) bool) bool {
	return slices.ContainsFunc(in, pred)
}

// AllMatch 所有元素满足 pred 返回 true（空切片返回 true，逻辑和数学一致）
func AllMatch[T any](in []T, pred func(T) bool) bool {
	for _, v := range in {
		if !pred(v) {
			return false
		}
	}
	return true
}

// NoneMatch 所有元素都不满足 pred（空切片返回 true）
func NoneMatch[T any](in []T, pred func(T) bool) bool {
	return !slices.ContainsFunc(in, pred)
}
