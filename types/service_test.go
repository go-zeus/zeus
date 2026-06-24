package types

import (
	"sort"
	"sync"
	"testing"
)

// TestNewServiceEntry 验证 NewServiceEntry 正确初始化 Clusters 和 Instances
func TestNewServiceEntry(t *testing.T) {
	s := NewServiceEntry()
	if s.Clusters == nil {
		t.Error("Clusters 不应为 nil")
	}
	if s.Instances == nil {
		t.Error("Instances 不应为 nil")
	}
	if len(s.Clusters) != 0 {
		t.Errorf("期望 Clusters 为空, 实际长度 = %d", len(s.Clusters))
	}
	if len(s.Instances) != 0 {
		t.Errorf("期望 Instances 为空, 实际长度 = %d", len(s.Instances))
	}
}

// TestServiceEntry_AddInstance 添加实例后验证 Instances 和 Cluster 中都包含
func TestServiceEntry_AddInstance(t *testing.T) {
	s := NewServiceEntry()
	ins := &Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}

	if err := s.AddInstance(ins); err != nil {
		t.Fatalf("添加实例失败: %v", err)
	}

	if _, ok := s.Instances["ins-1"]; !ok {
		t.Error("Instances 中应包含 ins-1")
	}
	if _, ok := s.Clusters["c1"]; !ok {
		t.Error("Clusters 中应包含 c1")
	}
	clusterIns := s.Clusters["c1"].GetInstances()
	if len(clusterIns) != 1 || clusterIns[0].ID != "ins-1" {
		t.Errorf("集群 c1 中应包含实例 ins-1")
	}
}

// TestServiceEntry_AddInstance_CreatesCluster 新集群名称会自动创建 Cluster
func TestServiceEntry_AddInstance_CreatesCluster(t *testing.T) {
	s := NewServiceEntry()
	ins := &Instance{ID: "ins-1", Name: "svc", Cluster: "new-cluster", IP: "127.0.0.1", Port: 8080}

	if err := s.AddInstance(ins); err != nil {
		t.Fatalf("添加实例失败: %v", err)
	}

	if _, ok := s.Clusters["new-cluster"]; !ok {
		t.Error("应自动创建集群 new-cluster")
	}
	if s.Clusters["new-cluster"].Name != "new-cluster" {
		t.Errorf("集群名称应为 new-cluster, 实际 = %s", s.Clusters["new-cluster"].Name)
	}
}

// TestServiceEntry_DelInstance 添加后删除实例，验证已移除
func TestServiceEntry_DelInstance(t *testing.T) {
	s := NewServiceEntry()
	ins := &Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}

	s.AddInstance(ins)
	s.DelInstance(ins)

	if _, ok := s.Instances["ins-1"]; ok {
		t.Error("删除后 Instances 中不应包含 ins-1")
	}
	// 删除后空 cluster 应被自动清理，避免长期运行累积空集合
	if _, ok := s.Clusters["c1"]; ok {
		t.Error("删除后空 cluster c1 应被自动清理")
	}
}

// TestServiceEntry_DelInstance_KeepsNonEmptyCluster 删除实例但 cluster 还有其他实例时应保留 cluster
func TestServiceEntry_DelInstance_KeepsNonEmptyCluster(t *testing.T) {
	s := NewServiceEntry()
	ins1 := &Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080}
	ins2 := &Instance{ID: "ins-2", Name: "svc", Cluster: "c1", IP: "127.0.0.2", Port: 8081}
	s.AddInstance(ins1)
	s.AddInstance(ins2)

	s.DelInstance(ins1)

	if _, ok := s.Clusters["c1"]; !ok {
		t.Error("cluster c1 还有 ins-2，不应被清理")
	}
	if c := s.Clusters["c1"]; c != nil && len(c.GetInstances()) != 1 {
		t.Errorf("cluster c1 应剩 1 个实例, 实际 = %d", len(c.GetInstances()))
	}
}

// TestServiceEntry_Reload 加载不同实例集后旧数据应消失
func TestServiceEntry_Reload(t *testing.T) {
	s := NewServiceEntry()
	s.AddInstance(&Instance{ID: "old-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080})

	newInstances := []*Instance{
		{ID: "new-1", Name: "svc", Cluster: "c2", IP: "127.0.0.2", Port: 8081},
		{ID: "new-2", Name: "svc", Cluster: "c2", IP: "127.0.0.3", Port: 8082},
	}
	s.Reload(newInstances)

	if _, ok := s.Instances["old-1"]; ok {
		t.Error("Reload 后旧实例 old-1 应被清除")
	}
	if _, ok := s.Instances["new-1"]; !ok {
		t.Error("Reload 后应包含新实例 new-1")
	}
	if _, ok := s.Clusters["c1"]; ok {
		t.Error("Reload 后旧集群 c1 应被清除")
	}
	if _, ok := s.Clusters["c2"]; !ok {
		t.Error("Reload 后应包含新集群 c2")
	}
}

// TestServiceEntry_Reload_Empty 用空列表 Reload 后 maps 应为空
func TestServiceEntry_Reload_Empty(t *testing.T) {
	s := NewServiceEntry()
	s.AddInstance(&Instance{ID: "ins-1", Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: 8080})

	s.Reload(nil)

	if len(s.Instances) != 0 {
		t.Errorf("Reload(nil) 后 Instances 应为空, 实际长度 = %d", len(s.Instances))
	}
	if len(s.Clusters) != 0 {
		t.Errorf("Reload(nil) 后 Clusters 应为空, 实际长度 = %d", len(s.Clusters))
	}
}

// TestServiceEntry_AllClusterName 添加不同集群的实例后验证返回所有集群名
func TestServiceEntry_AllClusterName(t *testing.T) {
	s := NewServiceEntry()
	instances := []*Instance{
		{ID: "ins-1", Name: "svc", Cluster: "alpha", IP: "127.0.0.1", Port: 8080},
		{ID: "ins-2", Name: "svc", Cluster: "beta", IP: "127.0.0.2", Port: 8081},
		{ID: "ins-3", Name: "svc", Cluster: "gamma", IP: "127.0.0.3", Port: 8082},
	}
	for _, ins := range instances {
		s.AddInstance(ins)
	}

	names := s.AllClusterName()
	sort.Strings(names)

	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("期望 %d 个集群名, 实际 = %d", len(want), len(names))
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("索引 %d: 期望 %s, 实际 %s", i, n, names[i])
		}
	}
}

// TestServiceEntry_Concurrent 并发操作验证无数据竞争
func TestServiceEntry_Concurrent(t *testing.T) {
	s := NewServiceEntry()
	var wg sync.WaitGroup

	// 并发添加
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ins := &Instance{ID: string(rune(n)), Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: n}
			_ = s.AddInstance(ins)
		}(i)
	}

	// 并发读取
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.AllClusterName()
		}()
	}

	// 并发删除
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ins := &Instance{ID: string(rune(n)), Name: "svc", Cluster: "c1", IP: "127.0.0.1", Port: n}
			s.DelInstance(ins)
		}(i)
	}

	wg.Wait()
}
