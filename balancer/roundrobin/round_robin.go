package roundrobin

import (
	"errors"
	"math/rand/v2"
	"sync/atomic"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/types"
)

// ErrNoInstances 表示当前 balancer 中没有可用实例
var ErrNoInstances = errors.New("balancer: no instances available")

type roundRobinBalancer struct {
	curIndex  uint64
	instances []*types.Instance
}

// New 创建轮询负载均衡器，返回 balancer.Balancer
func New() balancer.Balancer {
	return &roundRobinBalancer{}
}

// NewRoundRobin 创建轮询负载均衡器（兼容旧代码）
//
// Deprecated: 使用 New() 代替，未来版本移除。
func NewRoundRobin() balancer.Balancer {
	return New()
}

// Reload 重新加载实例列表，返回独立的 balancer 实例
//
// 必须返回新实例（而非修改 r 自身）以支持多 cluster 场景：
// 同一个 balancer 模板可派生多个独立 balancer，每个对应一个 cluster。
// 修改 r 自身会导致多 cluster 共享状态，引发并发问题。
func (r *roundRobinBalancer) Reload(ins []*types.Instance) balancer.Balancer {
	// 浅拷贝：避免调用方后续 append/修改底层数组影响 balancer 内部列表（防外部别名）
	cp := make([]*types.Instance, len(ins))
	copy(cp, ins)
	// 随机起始游标：避免服务发现频繁 Reload 后，新 balancer 首轮 Next 系统性偏向 instances[0]
	return &roundRobinBalancer{instances: cp, curIndex: rand.Uint64()}
}

func (r *roundRobinBalancer) Next() (*types.Instance, error) {
	if len(r.instances) == 0 {
		return nil, ErrNoInstances
	}
	n := atomic.AddUint64(&r.curIndex, 1)
	return r.instances[(n-1)%uint64(len(r.instances))], nil
}
