package types

import (
	"fmt"
	"github.com/go-zeus/zeus/metadata"
	"sync"
)

// Cluster 集群
//
// 表示同名 + 同 cluster 字段的实例集合，是路由层的灰度/单元化分组单元。
// 对齐 K8s Endpoints / Istio Cluster 概念。
type Cluster struct {
	Name      string               `json:"name"`
	Instances map[string]*Instance `json:"instances"`
	Labels    []string             `json:"labels"`
	Metadata  metadata.MD          `json:"metadata"`
	mu        sync.RWMutex
}

// NewCluster 创建一个指定名称的空 Cluster。
func NewCluster(name string) *Cluster {
	return &Cluster{Name: name, Instances: make(map[string]*Instance)}
}

// AddInstance 向 Cluster 添加一个实例。
//
// 若实例 ID 已存在则返回错误（防止重复注册）。
func (c *Cluster) AddInstance(ins *Instance) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.Instances[ins.ID]; ok {
		return fmt.Errorf("types: instance %q already exists in cluster %q", ins.ID, ins.Cluster)
	}
	c.Instances[ins.ID] = ins
	return nil
}

// DelInstance 从 Cluster 中删除指定实例（按 ID 匹配）。
//
// 删除不存在的实例是 no-op，不返回错误。
func (c *Cluster) DelInstance(ins *Instance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Instances, ins.ID)
}

// DelInstanceAndCount 删除实例并原子返回删除后剩余的实例数。
//
// 供调用方判断是否需要清理 cluster（避免 DelInstance + 单独查询之间的竞态）。
func (c *Cluster) DelInstanceAndCount(ins *Instance) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Instances, ins.ID)
	return len(c.Instances)
}

// GetInstances 返回 Cluster 中所有实例的切片。
//
// 返回顺序不定（map 迭代无序）。调用方不应修改返回的 Instance 内容。
func (c *Cluster) GetInstances() []*Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var data []*Instance
	for _, instance := range c.Instances {
		data = append(data, instance)
	}
	return data
}
