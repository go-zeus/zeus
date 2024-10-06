package types

import (
	"fmt"
	"sync"
)

// Service 服务
type Service struct {
	Name      string               `json:"name"`
	Clusters  map[string]*Cluster  `json:"clusters"`
	Instances map[string]*Instance `json:"instances"`
	mu        sync.RWMutex
}

func NewService() *Service {
	return &Service{Clusters: make(map[string]*Cluster), Instances: make(map[string]*Instance)}
}

func (s *Service) AddInstance(ins *Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Instances[ins.Cluster]; ok {
		return fmt.Errorf("service[%s]中实例[%s]已经存在", ins.Name, ins.Id)
	}
	s.Instances[ins.Id] = ins
	if _, ok := s.Clusters[ins.Cluster]; !ok {
		s.Clusters[ins.Cluster] = NewCluster(ins.Cluster)
	}
	return s.Clusters[ins.Cluster].AddInstance(ins)
}

func (s *Service) DelInstance(ins *Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Instances, ins.Id)
	if _, ok := s.Clusters[ins.Cluster]; !ok {
		s.Clusters[ins.Cluster] = NewCluster(ins.Cluster)
	}
	s.Clusters[ins.Cluster].DelInstance(ins)
}

func (s *Service) Reload(ins []*Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Instances = make(map[string]*Instance)
	for _, i := range ins {
		_ = s.AddInstance(i)
	}
	s.Clusters = map[string]*Cluster{}
	for _, i := range ins {
		if _, ok := s.Clusters[i.Cluster]; !ok {
			s.Clusters[i.Cluster] = NewCluster(i.Cluster)
		}
		_ = s.Clusters[i.Cluster].AddInstance(i)
	}
}

func (s *Service) AllClusterName() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var data []string
	for _, cluster := range s.Clusters {
		data = append(data, cluster.Name)
	}
	return data
}

func (s *Service) AllCluster() []*Cluster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var data []*Cluster
	for _, cluster := range s.Clusters {
		data = append(data, cluster)
	}
	return data
}
