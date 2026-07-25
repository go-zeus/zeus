package random

import (
	"errors"
	"math/rand/v2"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/types"
)

// ErrNoInstances 表示当前 balancer 中没有可用实例
var ErrNoInstances = errors.New("balancer: no instances available")

type randomBalancer struct {
	instances []*types.Instance
}

// New 创建随机负载均衡器，返回 balancer.Balancer
func New() balancer.Balancer {
	return &randomBalancer{}
}

// NewRandom 创建随机负载均衡器（兼容旧代码）
func NewRandom() balancer.Balancer {
	return New()
}

// Reload 重新加载实例列表，返回独立的 balancer 实例
//
// 必须返回新实例（而非修改 r 自身）以支持多 cluster 场景：
// 同一个 balancer 模板可派生多个独立 balancer，每个对应一个 cluster。
// 修改 r 自身会导致多 cluster 共享状态，引发并发问题。
// 该契约与 roundrobin.Reload 一致，被 client/proxy 调用方依赖。
func (r *randomBalancer) Reload(ins []*types.Instance) balancer.Balancer {
	// 浅拷贝：避免调用方后续 append/修改底层数组影响 balancer 内部列表（防外部别名）
	cp := make([]*types.Instance, len(ins))
	copy(cp, ins)
	return &randomBalancer{instances: cp}
}

func (r *randomBalancer) Next() (*types.Instance, error) {
	if len(r.instances) == 0 {
		return nil, ErrNoInstances
	}
	// math/rand/v2 顶层函数 per-goroutine lock-free（PCG），高并发优于 math/rand 全局锁
	return r.instances[rand.IntN(len(r.instances))], nil
}
