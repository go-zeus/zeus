package random

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

func TestRandomBalance_Next(t *testing.T) {
	lb := New()
	nodes := []*types.Instance{
		{ID: "1", Name: "111"},
		{ID: "2", Name: "222"},
		{ID: "3", Name: "333"},
	}
	// Reload 返回新实例，原 lb 不变（多 cluster 派生契约）
	lb = lb.Reload(nodes)
	node, _ := lb.Next()
	t.Log(node.Name)

	node, _ = lb.Next()
	t.Log(node.Name)

	node, _ = lb.Next()
	t.Log(node.Name)
}

func TestRandomBalance_Empty(t *testing.T) {
	lb := New()
	_, err := lb.Next()
	if err == nil {
		t.Fatal("expected error for empty instances")
	}
	if !errors.Is(err, ErrNoInstances) {
		t.Errorf("expected ErrNoInstances sentinel, got %v", err)
	}
}

// TestRandomBalance_ReloadReturnsNewInstance 验证 Reload 返回独立实例
// 多 cluster 场景下，必须返回新实例避免共享状态导致并发问题
func TestRandomBalance_ReloadReturnsNewInstance(t *testing.T) {
	lb := New()
	nodes1 := []*types.Instance{{ID: "1", Name: "a"}}
	lb2 := lb.Reload(nodes1)

	if lb == lb2 {
		t.Fatal("Reload should return a new instance, not modify self")
	}

	// 原 lb 仍为空实例集
	if _, err := lb.Next(); err == nil {
		t.Fatal("original balancer should remain empty after Reload returns new instance")
	}
	// 新 lb 有候选实例
	ins, err := lb2.Next()
	if err != nil {
		t.Fatalf("new balancer Next: %v", err)
	}
	if ins.ID != "1" {
		t.Errorf("got %s, want 1", ins.ID)
	}
}
