package roundrobin

import (
	"errors"
	"testing"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/types"
)

// 验证 New() 返回值满足 balancer.Balancer 接口
func TestNewImplementsBalancer(t *testing.T) {
	var _ balancer.Balancer = New()
}

func TestNewRoundRobin_Next(t *testing.T) {
	lb := New()
	nodes := []*types.Instance{
		{ID: "1", Name: "111"},
		{ID: "2", Name: "222"},
		{ID: "3", Name: "333"},
	}
	// Reload 返回新实例，应当用返回值继续调用
	lb = lb.Reload(nodes)
	node, _ := lb.Next()
	t.Log(node.Name)

	node, _ = lb.Next()
	t.Log(node.Name)

	node, _ = lb.Next()
	t.Log(node.Name)
}

func TestRoundRobin_Empty(t *testing.T) {
	lb := New()
	_, err := lb.Next()
	if err == nil {
		t.Fatal("expected error for empty instances")
	}
	if !errors.Is(err, ErrNoInstances) {
		t.Errorf("expected ErrNoInstances sentinel, got %v", err)
	}
}

// 并发基准测试
func BenchmarkNewRoundRobin_Next(b *testing.B) {
	lb := New()
	nodes := []*types.Instance{
		{ID: "1", Name: "111"},
		{ID: "2", Name: "222"},
		{ID: "3", Name: "333"},
	}
	lb = lb.Reload(nodes)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Next()
		}
	})
}
