package round_robin

import (
	"errors"
	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/types"
	"sync"
	"sync/atomic"
)

// 轮询负载均衡
type roundRobin struct {
	curIndex  int32
	instances []*types.Instance
	mu        sync.Mutex
}

func NewRoundRobin() *roundRobin {
	return &roundRobin{}
}

func (r *roundRobin) Reload(ins []*types.Instance) balancer.LoadBalance {
	r.instances = ins
	return r
}

func (r *roundRobin) Next() (*types.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.instances) == 0 {
		return nil, errors.New("not found node")
	}
	lens := int32(len(r.instances))
	if r.curIndex >= lens {
		r.curIndex = 0
	}
	node := r.instances[r.curIndex]
	var next = atomic.AddInt32(&r.curIndex, 1)
	r.curIndex = next % lens
	return node, nil
}
