package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-zeus/zeus/cache"
)

// cache/memory 是高频热路径（每个业务请求可能多次访问），本 bench 用于：
//   - 防回归：sync.Map + entry 分配开销变化能被立刻发现
//   - 量化 noop tracer/meter 注入的基线开销（默认装配路径）
//
// 所有 bench 均不注入 tracer/meter（走 noop 兜底），与 L1 默认装配一致。

// BenchmarkSet 写入路径（Store + entry 分配）
func BenchmarkSet(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, fmt.Sprintf("k-%d", i), i)
	}
}

// BenchmarkSet_WithTTL 带 TTL 的写入（额外计算 expireAt）
func BenchmarkSet_WithTTL(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, fmt.Sprintf("k-%d", i), i, cache.WithTTL(60*time.Second))
	}
}

// BenchmarkGet_Hit 命中读路径（典型热路径）
func BenchmarkGet_Hit(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	_ = c.Set(ctx, "k", "v")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(ctx, "k")
	}
}

// BenchmarkGet_Miss 未命中读路径（无 span 之外的额外开销）
func BenchmarkGet_Miss(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get(ctx, "not-exist")
	}
}

// BenchmarkHas 存在性检查（不返回 value，理论应略快于 Get）
func BenchmarkHas(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	_ = c.Set(ctx, "k", "v")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Has(ctx, "k")
	}
}

// BenchmarkDelete 删除路径
func BenchmarkDelete(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	// 预灌 N 个 key，循环删除（Delete 不存在的 key 是 no-op，开销稳定）
	const N = 10000
	for i := 0; i < N; i++ {
		_ = c.Set(ctx, fmt.Sprintf("k-%d", i), i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Delete(ctx, fmt.Sprintf("k-%d", i%N))
	}
}

// BenchmarkGet_Parallel 并发读（sync.Map 读多写少场景的强项）
func BenchmarkGet_Parallel(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	_ = c.Set(ctx, "k", "v")
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Get(ctx, "k")
		}
	})
}

// BenchmarkSet_Parallel 并发写（sync.Map 的弱项，用于量化写竞争开销）
func BenchmarkSet_Parallel(b *testing.B) {
	c := New().(*cacheImpl)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = c.Set(ctx, fmt.Sprintf("k-%d", i), i)
			i++
		}
	})
}
