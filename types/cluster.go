package types

import (
	"fmt"
	"github.com/go-zeus/zeus/metadata"
	"sync"
)

// Cluster 集群
type Cluster struct {
	Name      string               `json:"name"`
	Instances map[string]*Instance `json:"instances"`
	Labels    []string             `json:"labels"`
	Metadata  metadata.MD          `json:"metadata"`
	mu        sync.RWMutex
}

func NewCluster(name string) *Cluster {
	return &Cluster{Name: name, Instances: make(map[string]*Instance)}
}

func (c *Cluster) AddInstance(ins *Instance) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.Instances[ins.Id]; ok {
		return fmt.Errorf("cluster[%s]中实例[%s]已经存在", ins.Cluster, ins.Id)
	}
	c.Instances[ins.Id] = ins
	return nil
}

func (c *Cluster) DelInstance(ins *Instance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Instances, ins.Id)
}

func (c *Cluster) GetInstances() []*Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var data []*Instance
	for _, instance := range c.Instances {
		data = append(data, instance)
	}
	return data
}
