package roundrobin

import (
	"errors"
	"testing"

	"github.com/go-zeus/zeus/types"
)

// makeInstances 构造 N 个测试实例
func makeInstances(n int) []*types.Instance {
	out := make([]*types.Instance, n)
	for i := 0; i < n; i++ {
		out[i] = &types.Instance{
			ID:       "ins-" + itoa(i),
			Name:     "svc",
			Cluster:  "default",
			Protocol: "http",
			IP:       "10.0.0.1",
			Port:     8080 + i,
		}
	}
	return out
}

// itoa 简易 int → string（避免 strconv 引入）
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	buf := [16]byte{}
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// BenchmarkNext_Single 单实例轮询（最热路径）
func BenchmarkNext_Single(b *testing.B) {
	lb := New().Reload(makeInstances(1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lb.Next()
	}
}

// BenchmarkNext_Small 小规模（5 实例，典型生产规模）
func BenchmarkNext_Small(b *testing.B) {
	lb := New().Reload(makeInstances(5))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lb.Next()
	}
}

// BenchmarkNext_Large 大规模（100 实例，极端场景）
func BenchmarkNext_Large(b *testing.B) {
	lb := New().Reload(makeInstances(100))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lb.Next()
	}
}

// BenchmarkNext_Parallel 并发场景（验证 atomic 路径无竞争开销）
func BenchmarkNext_Parallel(b *testing.B) {
	lb := New().Reload(makeInstances(10))
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = lb.Next()
		}
	})
}

// BenchmarkReload Reload 派生新 balancer 的开销（每次服务发现刷新触发）
func BenchmarkReload(b *testing.B) {
	lb := New()
	ins := makeInstances(50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lb.Reload(ins)
	}
}

// BenchmarkNext_Empty 空实例错误路径
func BenchmarkNext_Empty(b *testing.B) {
	lb := New().Reload(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := lb.Next()
		if err == nil || !errors.Is(err, ErrNoInstances) {
			b.Fatal("expected ErrNoInstances")
		}
	}
}
