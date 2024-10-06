package random

import (
	"errors"
	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/types"
	"math/rand"
)

// 随机负载均衡
type random struct {
	instances []*types.Instance
}

func NewRandom() *random {
	return &random{}
}

func (r *random) Reload(ins []*types.Instance) balancer.LoadBalance {
	r.instances = ins
	return r
}

func (r *random) Next() (*types.Instance, error) {
	if len(r.instances) == 0 {
		return nil, errors.New("not found node")
	}
	return r.instances[rand.Intn(len(r.instances))], nil
}
