package registry

import "github.com/go-zeus/zeus/types"

// Storage 集群缓存
type Storage interface {
	GetService(serviceName string) *types.Service
	GetCluster(serviceName, clusterName string) *types.Cluster
	Reload(ins []*types.Instance) error
	AllService() []*types.Service
	AllServiceName() []string
	AllClusterName(serviceName string) []string
	Exists(serviceName string) bool
}
