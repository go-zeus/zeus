package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-zeus/zeus/types"
)

// BenchmarkRegister 注册路径（写锁 + 通知 watcher）
func BenchmarkRegister(b *testing.B) {
	m := NewMemory().(*memory)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Register(context.Background(), &types.Instance{
			ID:      fmt.Sprintf("ins-%d", i),
			Name:    "svc",
			Cluster: "default",
			IP:      "10.0.0.1",
			Port:    9000 + i,
		})
	}
}

// BenchmarkGetService_Hit 命中路径（典型读）
func BenchmarkGetService_Hit(b *testing.B) {
	m := NewMemory().(*memory)
	_ = m.Register(context.Background(), &types.Instance{
		ID: "ins-1", Name: "svc", Cluster: "default", IP: "10.0.0.1", Port: 8080,
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.GetService(context.Background(), "svc")
	}
}

// BenchmarkGetService_Miss 未命中路径
func BenchmarkGetService_Miss(b *testing.B) {
	m := NewMemory().(*memory)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.GetService(context.Background(), "not-exist")
	}
}

// BenchmarkDeregister 注销路径
func BenchmarkDeregister(b *testing.B) {
	m := NewMemory().(*memory)
	// 预注册 N 个实例
	const N = 1000
	for i := 0; i < N; i++ {
		_ = m.Register(context.Background(), &types.Instance{
			ID: fmt.Sprintf("ins-%d", i), Name: "svc", Cluster: "default",
			IP: "10.0.0.1", Port: 8080 + i,
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Deregister(context.Background(), &types.Instance{
			ID: fmt.Sprintf("ins-%d", i%N), Name: "svc",
		})
	}
}

// BenchmarkRegister_GetService_Mix 读写混合（80% 读 / 20% 写）
func BenchmarkRegister_GetService_Mix(b *testing.B) {
	m := NewMemory().(*memory)
	_ = m.Register(context.Background(), &types.Instance{
		ID: "ins-1", Name: "svc", Cluster: "default", IP: "10.0.0.1", Port: 8080,
	})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%5 == 0 {
				_ = m.Register(context.Background(), &types.Instance{
					ID: fmt.Sprintf("ins-par-%d", i), Name: "svc", Cluster: "default",
					IP: "10.0.0.1", Port: 9000 + i,
				})
			} else {
				_, _ = m.GetService(context.Background(), "svc")
			}
			i++
		}
	})
}
