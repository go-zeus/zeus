package memory

import (
	"context"
	"sync"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/types"
)

// memory 内存注册中心实现，同时实现 Registrar 和 Discovery 接口
type memory struct {
	services map[string]*types.ServiceEntry
	mu       sync.RWMutex
	watchers map[chan struct{}]struct{} // watcher 集合（O(1) 注销）
	wmu      sync.Mutex
	closed   bool
}

// New 创建内存注册中心实例，返回 registry.Registrar
// 返回值同时实现了 registry.Discovery 接口
func New() registry.Registrar {
	return &memory{
		services: map[string]*types.ServiceEntry{},
		watchers: map[chan struct{}]struct{}{},
	}
}

// NewMemory 创建内存注册中心实例（兼容旧代码）
func NewMemory() registry.Registrar {
	return New()
}

func (m *memory) Register(_ context.Context, ins *types.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[ins.Name]; !ok {
		m.services[ins.Name] = types.NewServiceEntry()
	}
	// AddInstance 在重复 id 时返回 error，需向上传递（避免静默吞错）
	if err := m.services[ins.Name].AddInstance(ins); err != nil {
		return err
	}
	m.notifyWatchers()
	return nil
}

func (m *memory) Deregister(_ context.Context, ins *types.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[ins.Name]; !ok {
		// 服务不存在视为已注销，幂等返回 nil（与 K8s delete 等行为一致）
		return nil
	}
	m.services[ins.Name].DelInstance(ins)
	m.notifyWatchers()
	return nil
}

// Watch 订阅服务变更事件，返回事件 channel 和注销函数
// channel 在 registry 关闭时会被关闭（调用方应配合 ctx.Done() 使用）
func (m *memory) Watch(_ context.Context, _ string) (<-chan struct{}, error) {
	ch := make(chan struct{}, 1)
	m.wmu.Lock()
	if m.closed {
		m.wmu.Unlock()
		close(ch)
		return ch, nil
	}
	m.watchers[ch] = struct{}{}
	m.wmu.Unlock()
	return ch, nil
}

// Close 关闭所有 watcher channel 并标记 registry 为已关闭
// 调用方应在应用关闭时调用，避免 watcher goroutine 泄漏
func (m *memory) Close() {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	for ch := range m.watchers {
		close(ch)
		delete(m.watchers, ch)
	}
}

// notifyWatchers 通知所有 watcher，采用 fan-out 模式分发事件
func (m *memory) notifyWatchers() {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	for ch := range m.watchers {
		// 清空旧事件，确保最新一次触发能被接收
		select {
		case <-ch:
		default:
		}
		// 非阻塞写入；watcher 已退出消费时丢弃事件，避免阻塞注册流程
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (m *memory) GetService(_ context.Context, serviceName string) (*types.ServiceEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.services[serviceName], nil
}
