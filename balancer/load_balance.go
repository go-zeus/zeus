package balancer

import (
	"github.com/go-zeus/zeus/types"
)

type LoadBalance interface {
	Next() (*types.Instance, error)
	Reload([]*types.Instance) LoadBalance
}
