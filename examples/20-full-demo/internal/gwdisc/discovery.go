// Package gwdisc 把 gateway 的 /api/services HTTP 接口适配为 zeus registry.Discovery + Watcher。
//
// 设计：
//   - 后台 goroutine 每 2s 拉一次 gateway 服务列表，对比签名变化时通知所有 watcher
//   - GetService 返回当前缓存（带本地缓存避免每次 HTTP 调用）
//   - 实现 Watcher 接口让 zeus client 能自动 reload 实例变化
package gwdisc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

const pollInterval = 2 * time.Second

// httpDiscovery HTTP 拉取 gateway 服务列表的 Discovery 实现
type httpDiscovery struct {
	gatewayURL string
	http       *http.Client

	mu       sync.RWMutex
	services map[string]*types.ServiceEntry // 当前缓存（按 name 索引）
	sig      string                         // 上次拉取的签名（用于变更检测）

	wmu      sync.Mutex
	watchers []chan struct{}
}

// New 创建 HTTP Discovery（后台启动轮询 goroutine）
// gatewayURL 形如 "http://gateway:8080"
func New(gatewayURL string) registry.Discovery {
	d := &httpDiscovery{
		gatewayURL: gatewayURL,
		http:       &http.Client{Timeout: 3 * time.Second},
		services:   make(map[string]*types.ServiceEntry),
	}
	go d.pollLoop()
	return d
}

// 编译期检查实现 Discovery + Watcher
var (
	_ registry.Discovery = (*httpDiscovery)(nil)
	_ registry.Watcher   = (*httpDiscovery)(nil)
)

// GetService 从本地缓存返回服务（缓存由后台 pollLoop 维护）
func (d *httpDiscovery) GetService(_ context.Context, name string) (*types.ServiceEntry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	entry, ok := d.services[name]
	if !ok {
		// 返回空 entry（而不是 nil），避免下游 nil panic
		return types.NewServiceEntry(), nil
	}
	return entry, nil
}

// Watch 订阅服务变更事件（channel 收到 struct{}{} 表示有变化，需重新 GetService）
func (d *httpDiscovery) Watch(_ context.Context, _ string) (<-chan struct{}, error) {
	ch := make(chan struct{}, 1)
	d.wmu.Lock()
	d.watchers = append(d.watchers, ch)
	d.wmu.Unlock()
	return ch, nil
}

// pollLoop 后台轮询 gateway，签名变化时通知所有 watcher
func (d *httpDiscovery) pollLoop() {
	// 启动时立即拉一次
	d.poll()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		d.poll()
	}
}

// poll 单次拉取 + 对比签名 + 通知
func (d *httpDiscovery) poll() {
	body, err := d.fetch()
	if err != nil {
		return // gateway 尚未就绪，下次再试
	}

	// 签名对比：避免无变化时重复通知
	newSig := signature(body)
	d.mu.RLock()
	oldSig := d.sig
	d.mu.RUnlock()
	if newSig == oldSig {
		return
	}

	// 组装新缓存
	newCache := make(map[string]*types.ServiceEntry)
	for name, instances := range body.Services {
		entry := types.NewServiceEntry()
		entry.Name = name
		for _, raw := range instances {
			cluster := raw.Cluster
			if cluster == "" {
				cluster = routing.Default
			}
			_ = entry.AddInstance(&types.Instance{
				ID:       raw.ID,
				Name:     raw.Name,
				Cluster:  cluster,
				Protocol: raw.Protocol,
				IP:       raw.IP,
				Port:     raw.Port,
			})
		}
		newCache[name] = entry
	}

	d.mu.Lock()
	d.services = newCache
	d.sig = newSig
	d.mu.Unlock()

	d.notify()
}

// fetch HTTP 拉取服务列表
func (d *httpDiscovery) fetch() (*gwapi.ServicesResponse, error) {
	req, _ := http.NewRequest(http.MethodGet, d.gatewayURL+"/api/services", nil)
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gwdisc: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gwdisc: http %d", resp.StatusCode)
	}
	var body gwapi.ServicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("gwdisc: decode: %w", err)
	}
	return &body, nil
}

// notify 通知所有 watcher（fan-out，清空旧事件保证最新一次能接收）
func (d *httpDiscovery) notify() {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	for _, ch := range d.watchers {
		select {
		case <-ch:
		default:
		}
		ch <- struct{}{}
	}
}

// signature 计算服务列表签名（排序后的 id 列表）
func signature(resp *gwapi.ServicesResponse) string {
	names := make([]string, 0, len(resp.Services))
	for name := range resp.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	var ids []string
	for _, name := range names {
		ins := resp.Services[name]
		sort.Slice(ins, func(i, j int) bool { return ins[i].ID < ins[j].ID })
		for _, i := range ins {
			ids = append(ids, i.ID)
		}
	}
	return strings.Join(ids, ",")
}
