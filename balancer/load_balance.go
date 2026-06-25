package balancer

import "github.com/go-zeus/zeus/types"

// Balancer 负载均衡器接口
type Balancer interface {
	Next() (*types.Instance, error)
	Reload([]*types.Instance) Balancer
}
