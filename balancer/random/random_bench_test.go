package random

import (
	"testing"

	"github.com/go-zeus/zeus/types"
)

func makeInstances(n int) []*types.Instance {
	out := make([]*types.Instance, n)
	for i := 0; i < n; i++ {
		out[i] = &types.Instance{
			ID:      "ins-" + itoa(i),
			Name:    "svc",
			Cluster: "default",
			IP:      "10.0.0.1",
			Port:    8080 + i,
		}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	buf := [16]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// BenchmarkNext_Single 单实例
func BenchmarkNext_Single(b *testing.B) {
	lb := New().Reload(makeInstances(1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lb.Next()
	}
}

// BenchmarkNext_Small 小规模（5 实例）
func BenchmarkNext_Small(b *testing.B) {
	lb := New().Reload(makeInstances(5))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lb.Next()
	}
}

// BenchmarkNext_Large 大规模（100 实例）
func BenchmarkNext_Large(b *testing.B) {
	lb := New().Reload(makeInstances(100))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lb.Next()
	}
}

// BenchmarkNext_Parallel 并发场景
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

// BenchmarkReload Reload 开销
func BenchmarkReload(b *testing.B) {
	lb := New()
	ins := makeInstances(50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lb.Reload(ins)
	}
}
