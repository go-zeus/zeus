package types

import (
	"sync"
	"testing"
)

// TestNewCluster 验证 NewCluster 正确初始化 Name 和 Instances
func TestNewCluster(t *testing.T) {
	c := NewCluster("test-cluster")
	if c.Name != "test-cluster" {
		t.Errorf("期望 Name = test-cluster, 实际 = %s", c.Name)
	}
	if c.Instances == nil {
		t.Error("Instances 不应为 nil")
	}
	if len(c.Instances) != 0 {
		t.Errorf("期望 Instances 为空, 实际长度 = %d", len(c.Instances))
	}
}

// TestCluster_AddInstance 添加实例后验证 GetInstances 包含该实例
func TestCluster_AddInstance(t *testing.T) {
	c := NewCluster("c1")
	ins := &Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}

	if err := c.AddInstance(ins); err != nil {
		t.Fatalf("添加实例失败: %v", err)
	}

	got := c.GetInstances()
	if len(got) != 1 {
		t.Fatalf("期望 1 个实例, 实际 = %d", len(got))
	}
	if got[0].ID != "ins-1" {
		t.Errorf("期望实例 Id = ins-1, 实际 = %s", got[0].ID)
	}
}

// TestCluster_AddInstance_Duplicate 重复添加同一实例应返回错误
func TestCluster_AddInstance_Duplicate(t *testing.T) {
	c := NewCluster("c1")
	ins := &Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}

	if err := c.AddInstance(ins); err != nil {
		t.Fatalf("首次添加实例失败: %v", err)
	}
	if err := c.AddInstance(ins); err == nil {
		t.Error("重复添加实例应返回错误, 但返回 nil")
	}
}

// TestCluster_DelInstance 添加后删除实例，验证已移除
func TestCluster_DelInstance(t *testing.T) {
	c := NewCluster("c1")
	ins := &Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}

	c.AddInstance(ins)
	c.DelInstance(ins)

	got := c.GetInstances()
	if len(got) != 0 {
		t.Errorf("删除后期望 0 个实例, 实际 = %d", len(got))
	}
}

// TestCluster_DelInstance_NonExisting 删除不存在的实例不应 panic
func TestCluster_DelInstance_NonExisting(t *testing.T) {
	c := NewCluster("c1")
	ins := &Instance{ID: "nonexist", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}

	// 不应 panic
	c.DelInstance(ins)

	if len(c.Instances) != 0 {
		t.Errorf("期望 0 个实例, 实际 = %d", len(c.Instances))
	}
}

// TestCluster_GetInstances_Empty 新集群返回空切片或 nil
func TestCluster_GetInstances_Empty(t *testing.T) {
	c := NewCluster("c1")
	got := c.GetInstances()
	// Go 中未 append 的 slice 为 nil，属于正常行为
	if len(got) != 0 {
		t.Errorf("期望长度为 0, 实际长度 = %d", len(got))
	}
}

// TestCluster_Concurrent 并发增删查，验证无数据竞争
func TestCluster_Concurrent(t *testing.T) {
	c := NewCluster("c1")
	var wg sync.WaitGroup

	// 并发添加
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ins := &Instance{ID: string(rune(n)), Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: n}
			_ = c.AddInstance(ins)
		}(i)
	}

	// 并发读取
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.GetInstances()
		}()
	}

	// 并发删除
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ins := &Instance{ID: string(rune(n)), Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: n}
			c.DelInstance(ins)
		}(i)
	}

	wg.Wait()
}
