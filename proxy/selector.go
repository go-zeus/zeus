package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

// staticSelector 静态后端选择器
// 始终返回固定的目标 URL，适合单实例场景
type staticSelector struct {
	target *url.URL
}

// NewStaticSelector 创建静态后端选择器
// target 为后端服务地址，如 http://127.0.0.1:9000
func NewStaticSelector(target *url.URL) Selector {
	return &staticSelector{target: target}
}

func (s *staticSelector) Pick(_ *http.Request) (*url.URL, error) {
	return s.target, nil
}

// discoverySelector 动态服务发现选择器
// 集成 registry.Discovery + balancer.Balancer + 集群路由（X-Zeus-Cluster）
//
// 刷新策略：基于实例签名（排序后的 ID 列表）判断 cluster 是否变化
// 仅在签名变化时调用 balancer.Reload 派生新 balancer，避免无谓重建
// 这保证 balancer 的内部状态（如 roundrobin 的 curIndex）在实例稳定时持续生效
//
// 并发模型：Pick 全程持锁，串行化 refresh + lb.Next()
// v1 优先保证正确性，吞吐可通过后台 watcher 模式优化（参考 client 包）
type discoverySelector struct {
	name        string
	dis         registry.Discovery
	lb          balancer.Balancer
	mu          sync.Mutex
	clusters    map[string]balancer.Balancer
	clusterSigs map[string]string
}

// NewDiscoverySelector 创建动态服务发现选择器
//
// name 为服务名称，dis 提供服务实例查询，lb 为负载均衡器模板
// （每个集群会基于 lb.Reload 派生独立的 Balancer）
//
// 路由规则：
//  1. 从 X-Zeus-Cluster header 解析集群标记
//  2. 优先路由到对应集群；若不存在则回退到 default 集群
//  3. 调用 cluster.Balancer.Next() 选择实例
func NewDiscoverySelector(name string, dis registry.Discovery, lb balancer.Balancer) Selector {
	return &discoverySelector{
		name:        name,
		dis:         dis,
		lb:          lb,
		clusters:    make(map[string]balancer.Balancer),
		clusterSigs: make(map[string]string),
	}
}

func (d *discoverySelector) Pick(r *http.Request) (*url.URL, error) {
	if d.dis == nil {
		return nil, fmt.Errorf("proxy: discovery is nil")
	}

	// 拉取最新服务实例
	srv, err := d.dis.GetService(r.Context(), d.name)
	if err != nil {
		return nil, fmt.Errorf("proxy: get service %q: %w", d.name, err)
	}
	if srv == nil {
		return nil, fmt.Errorf("proxy: service %q not found", d.name)
	}

	// 全程持锁：保证 refresh + Next 的原子性，避免 balancer 状态竞争
	d.mu.Lock()
	defer d.mu.Unlock()

	d.refreshClustersLocked(srv)

	// 集群路由
	c := routing.ClusterFromHTTPHeader(r.Header)

	lb, ok := d.clusters[c]
	if !ok {
		lb, ok = d.clusters[routing.Default]
	}
	if !ok {
		return nil, fmt.Errorf("proxy: no available cluster for %q (tried %q and default)", d.name, c)
	}

	ins, err := lb.Next()
	if err != nil {
		return nil, fmt.Errorf("proxy: pick instance: %w", err)
	}

	target := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", ins.IP, ins.Port),
	}
	return target, nil
}

// refreshClustersLocked 根据最新 ServiceEntry 实例刷新集群负载均衡器
// 仅在 cluster 实例签名变化时调用 balancer.Reload，避免重置 balancer 状态
// 调用方必须持有 d.mu
func (d *discoverySelector) refreshClustersLocked(srv *types.ServiceEntry) {
	newClusters := make(map[string]balancer.Balancer)
	newSigs := make(map[string]string)
	for name, cl := range srv.Clusters {
		instances := cl.GetInstances()
		if len(instances) == 0 || d.lb == nil {
			continue
		}
		sig := signature(instances)
		// 签名一致：复用现有 balancer，保留其内部状态（如 roundrobin 计数）
		if existing, ok := d.clusters[name]; ok && d.clusterSigs[name] == sig {
			newClusters[name] = existing
		} else {
			// 签名变化或新 cluster：派生新 balancer
			newClusters[name] = d.lb.Reload(instances)
		}
		newSigs[name] = sig
	}
	d.clusters = newClusters
	d.clusterSigs = newSigs
}

// signature 计算实例集合签名（排序后的 ID 列表）。
//
// 典型场景：刷新负载均衡实例池前比较签名，避免无变化的重复 Reload。
func signature(instances []*types.Instance) string {
	ids := make([]string, 0, len(instances))
	for _, ins := range instances {
		ids = append(ids, ins.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// 编译期检查两种 selector 实现 Selector 接口
var (
	_ Selector = (*staticSelector)(nil)
	_ Selector = (*discoverySelector)(nil)
)
