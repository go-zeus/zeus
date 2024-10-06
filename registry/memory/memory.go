package memory

import (
	"fmt"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/types"
	"github.com/go-zeus/zeus/utils/event"
	"sync"
)

type memory struct {
	services map[string]*types.Service
	mu       sync.RWMutex
	e        event.Event
}

func NewMemory() registry.Registry {
	return &memory{services: map[string]*types.Service{}, e: event.NewEvent()}
}

func (m *memory) Register(ins *types.Instance) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[ins.Name]; !ok {
		m.services[ins.Name] = types.NewService()
	}
	m.services[ins.Name].AddInstance(ins)
	return nil
}

func (m *memory) Deregister(ins *types.Instance) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[ins.Name]; !ok {
		return fmt.Errorf("服务[%s]不存在", ins.Name)
	}
	m.services[ins.Name].DelInstance(ins)
	return nil
}

func (m *memory) Watch(serviceName string) <-chan struct{} {
	return m.e.Watch()
}

func (m *memory) GetService(serviceName string) *types.Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.services[serviceName]
}

func (m *memory) GetCluster(serviceName, clusterName string) *types.Cluster {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if service, ok := m.services[serviceName]; ok {
		return service.Clusters[clusterName]
	}
	return nil
}

func (m *memory) Reload(ins []*types.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services = map[string]*types.Service{}
	var errIds []string
	for _, i := range ins {
		err := m.Register(i)
		if err != nil {
			errIds = append(errIds, i.Id)
		}
	}
	if len(errIds) > 0 {
		return fmt.Errorf("instances[%v] reload 失败", errIds)
	}
	return nil
}

func (m *memory) AllService() []*types.Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var services []*types.Service
	for _, service := range m.services {
		services = append(services, service)
	}
	return services
}

func (m *memory) AllServiceName() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var serviceNames []string
	for serviceName := range m.services {
		serviceNames = append(serviceNames, serviceName)
	}
	return serviceNames
}

func (m *memory) AllClusterName(serviceName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	service := m.GetService(serviceName)
	if service == nil {
		return []string{}
	}
	return service.AllClusterName()
}

func (m *memory) Exists(serviceName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.services[serviceName]
	return ok
}

func (m *memory) String() string {
	return "MemoryRegistry"
}
