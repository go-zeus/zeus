package types

import (
	"fmt"
	"sync"
)

// ServiceEntry 服务条目（逻辑服务 = 同名实例的集合）
//
// 命名说明：与 service 包（运行时 Service 概念）做名称区分，
// ServiceEntry 表示注册中心视角下的"逻辑服务"，对齐 K8s/Istio 的 ServiceEntry 概念。
type ServiceEntry struct {
	Name      string               `json:"name"`
	Clusters  map[string]*Cluster  `json:"clusters"`
	Instances map[string]*Instance `json:"instances"`
	mu        sync.RWMutex
}

// NewServiceEntry 创建一个空的 ServiceEntry。
//
// 内部已初始化 Clusters 和 Instances 映射，调用方无需再初始化。
func NewServiceEntry() *ServiceEntry {
	return &ServiceEntry{Clusters: make(map[string]*Cluster), Instances: make(map[string]*Instance)}
}

// AddInstance 向逻辑服务添加一个实例。
//
// 同步加入到 Instances 索引与对应 Cluster（cluster 不存在时自动创建）。
// 若实例 ID 已存在返回错误，由调用方决定如何处理（通常是上游事件重复）。
func (s *ServiceEntry) AddInstance(ins *Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Instances[ins.ID]; ok {
		return fmt.Errorf("types: instance %q already exists in service %q", ins.ID, ins.Name)
	}
	s.Instances[ins.ID] = ins
	if _, ok := s.Clusters[ins.Cluster]; !ok {
		s.Clusters[ins.Cluster] = NewCluster(ins.Cluster)
	}
	return s.Clusters[ins.Cluster].AddInstance(ins)
}

// DelInstance 从逻辑服务中移除指定实例。
//
// 同时从 Instances 索引与所属 Cluster 中删除；若 Cluster 删除后变空则一并清理，
// 避免长期运行累积空集合。
func (s *ServiceEntry) DelInstance(ins *Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Instances, ins.ID)
	if c, ok := s.Clusters[ins.Cluster]; ok {
		// 原子删除并返回剩余实例数，为 0 时清理空 cluster（避免长期运行累积空集合）
		if c.DelInstanceAndCount(ins) == 0 {
			delete(s.Clusters, ins.Cluster)
		}
	}
}

// Reload 用给定实例列表全量重建 ServiceEntry 的索引。
//
// 调用前已有的 Instances 与 Clusters 会被清空。
// 主要用于订阅注册中心全量推送场景（如 watcher 首次拉取）。
func (s *ServiceEntry) Reload(ins []*Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Instances = make(map[string]*Instance)
	s.Clusters = make(map[string]*Cluster)
	for _, i := range ins {
		// Reload 是全量重建，正常情况下不会有重复 id；
		// 若上游传入了重复 id（异常情况），以最后一条为准并记录到 Instances（覆盖式）
		s.Instances[i.ID] = i
		if _, ok := s.Clusters[i.Cluster]; !ok {
			s.Clusters[i.Cluster] = NewCluster(i.Cluster)
		}
		_ = s.Clusters[i.Cluster].AddInstance(i)
	}
}

// AllClusterName 返回当前逻辑服务下所有 Cluster 的名称列表。
//
// 典型场景：路由层枚举可选 cluster（如灰度发布、单元化路由场景）。
// 返回顺序不定（map 迭代无序）。
func (s *ServiceEntry) AllClusterName() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var data []string
	for _, cluster := range s.Clusters {
		data = append(data, cluster.Name)
	}
	return data
}

// AllCluster 返回当前逻辑服务下所有 Cluster 的指针列表。
//
// 返回的是 Cluster 引用的浅拷贝切片，调用方不应修改返回的 Cluster 内容。
// 返回顺序不定（map 迭代无序）。
func (s *ServiceEntry) AllCluster() []*Cluster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var data []*Cluster
	for _, cluster := range s.Clusters {
		data = append(data, cluster)
	}
	return data
}

// 兼容 alias：保留旧名称 Service 和 NewService 一个版本，便于下游平滑迁移
//
// Deprecated: 使用 ServiceEntry / NewServiceEntry 代替
type Service = ServiceEntry

// NewService 兼容旧调用方
//
// Deprecated: 使用 NewServiceEntry 代替
func NewService() *ServiceEntry {
	return NewServiceEntry()
}
