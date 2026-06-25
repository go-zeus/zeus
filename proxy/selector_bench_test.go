package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/types"
)

// BenchmarkStaticSelector_Pick 静态选择器（最热路径，应近零开销）
func BenchmarkStaticSelector_Pick(b *testing.B) {
	target, _ := url.Parse("http://127.0.0.1:9000")
	sel := NewStaticSelector(target)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.Pick(req)
	}
}

// BenchmarkDiscoverySelector_SingleCluster 单 cluster + 5 实例
func BenchmarkDiscoverySelector_SingleCluster(b *testing.B) {
	reg := memory.NewMemory()
	dis := reg.(registry.Discovery)
	for i := 0; i < 5; i++ {
		_ = reg.Register(context.Background(), &types.Instance{
			ID: fmt.Sprintf("ins-%d", i), Name: "svc", Cluster: "default",
			IP: "10.0.0.1", Port: 8080 + i,
		})
	}
	sel := NewDiscoverySelector("svc", dis, roundrobin.New())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.Pick(req)
	}
}

// BenchmarkDiscoverySelector_MultiCluster 多 cluster 路由
func BenchmarkDiscoverySelector_MultiCluster(b *testing.B) {
	reg := memory.NewMemory()
	dis := reg.(registry.Discovery)
	for i := 0; i < 15; i++ {
		c := fmt.Sprintf("cluster-%d", i%3)
		_ = reg.Register(context.Background(), &types.Instance{
			ID: fmt.Sprintf("ins-%d", i), Name: "svc", Cluster: c,
			IP: "10.0.0.1", Port: 8080 + i,
		})
	}
	sel := NewDiscoverySelector("svc", dis, roundrobin.New())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Zeus-Cluster", "cluster-1")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.Pick(req)
	}
}

// BenchmarkDiscoverySelector_LargeCluster 大规模 cluster（50 实例）
func BenchmarkDiscoverySelector_LargeCluster(b *testing.B) {
	reg := memory.NewMemory()
	dis := reg.(registry.Discovery)
	for i := 0; i < 50; i++ {
		_ = reg.Register(context.Background(), &types.Instance{
			ID: fmt.Sprintf("ins-%d", i), Name: "svc", Cluster: "default",
			IP: "10.0.0.1", Port: 8080 + i,
		})
	}
	sel := NewDiscoverySelector("svc", dis, roundrobin.New())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.Pick(req)
	}
}

// BenchmarkSignature 实例签名计算（每次刷新触发）
func BenchmarkSignature(b *testing.B) {
	ins := make([]*types.Instance, 20)
	for i := 0; i < 20; i++ {
		ins[i] = &types.Instance{ID: fmt.Sprintf("ins-%d", i)}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = signature(ins)
	}
}
