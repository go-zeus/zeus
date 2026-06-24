package balancer

import (
	"testing"

	"github.com/go-zeus/zeus/types"
)

// 验证 mock 类型满足 Balancer 接口
type mockBalancer struct {
	ins []*types.Instance
	idx int
}

func (m *mockBalancer) Next() (*types.Instance, error) {
	if len(m.ins) == 0 {
		return nil, nil
	}
	ins := m.ins[m.idx%len(m.ins)]
	m.idx++
	return ins, nil
}

func (m *mockBalancer) Reload(ins []*types.Instance) Balancer {
	m.ins = ins
	m.idx = 0
	return m
}

func TestBalancerInterface(t *testing.T) {
	var _ Balancer = &mockBalancer{}
}

func TestBalancerReload(t *testing.T) {
	instances := []*types.Instance{
		{ID: "inst-1", Name: "svc-a", IP: "10.0.0.1", Port: 8080},
		{ID: "inst-2", Name: "svc-a", IP: "10.0.0.2", Port: 8080},
	}

	lb := &mockBalancer{}

	// Reload 后 Next 应返回正确的实例
	lb.Reload(instances)

	ins, err := lb.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if ins == nil {
		t.Fatal("Next returned nil instance")
	}
	if ins.ID != "inst-1" {
		t.Errorf("Next() Id = %q, want %q", ins.ID, "inst-1")
	}

	ins2, err := lb.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if ins2 == nil {
		t.Fatal("Next returned nil instance")
	}
	if ins2.ID != "inst-2" {
		t.Errorf("Next() Id = %q, want %q", ins2.ID, "inst-2")
	}

	// 再次 Reload 为新实例列表
	newInstances := []*types.Instance{
		{ID: "inst-3", Name: "svc-b", IP: "10.0.1.1", Port: 9090},
	}
	lb.Reload(newInstances)

	ins3, err := lb.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if ins3.ID != "inst-3" {
		t.Errorf("After Reload, Next() Id = %q, want %q", ins3.ID, "inst-3")
	}
}
