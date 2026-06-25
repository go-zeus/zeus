// Package testutil 提供测试辅助工具。
//
// 设计目标：
//   - 集成测试基础设施：waitUntil / freePort / mustListen
//   - HTTP 测试服务器构建器（含就绪检查）
//   - mock registry / balancer（单元测试 mock 用）
//   - 断言辅助（assertEqual / assertPanic / requireNoError）
//
// 与标准 testing 包的关系：
//   - testing：基本断言（t.Fatal / t.Errorf）
//   - 本包：高频场景封装（等待条件 / 取空闲端口 / 临时服务）
//
// 注意：本包仅用于 _test.go 文件，不应被业务代码引用
package testutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/types"
)

// —— 等待 / 重试 ——

// WaitUntil 轮询等待条件成立，超时返回 error。
//
// 参数：
//   - timeout：总超时时间（从调用时刻起算）
//   - interval：每次轮询 cond 的间隔（建议 50-200ms，平衡灵敏度与 CPU 开销）
//   - cond：条件回调，返回 true 表示条件已满足
//
// 返回值：
//   - nil：条件在超时前成立
//   - error：超时仍未成立（错误消息含 timeout 时长，便于排查）
//
// 行为细节：
//   - 首次调用 cond 立即执行（不等待第一个 interval）
//   - 超时后再补一次 cond 检查，避免边界竞态（最后一次刚好成立）
//
// 用法：
//
//	err := testutil.WaitUntil(time.Second, 100*time.Millisecond, func() bool {
//	    return server.ReqCount() > 0
//	})
//	if err != nil { t.Fatal("server did not receive request") }
func WaitUntil(timeout, interval time.Duration, cond func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(interval)
	}
	if cond() {
		return nil
	}
	return fmt.Errorf("condition not met within %s", timeout)
}

// MustWaitUntil 等待条件满足，超时直接 t.Fatal
func MustWaitUntil(t *testing.T, timeout, interval time.Duration, cond func() bool, msg string) {
	t.Helper()
	if err := WaitUntil(timeout, interval, cond); err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}

// Poll 持续轮询直到 ctx 取消或 fn 返回 nil
//
// fn 返回 error 表示"未满足"，会继续重试
// fn 返回 nil 表示"已满足"，立即返回 nil
func Poll(ctx context.Context, interval time.Duration, fn func() error) error {
	if fn() == nil {
		return nil
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := fn(); err == nil {
				return nil
			}
		}
	}
}

// —— 端口 ——

// FreePort 取一个空闲端口（OS 分配，立即可用）
func FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// MustFreePort 取空闲端口，失败 t.Fatal
func MustFreePort(t *testing.T) int {
	t.Helper()
	p, err := FreePort()
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	return p
}

// —— HTTP 测试服务器 ——

// HTTPServer 包装 httptest.Server，提供就绪检查
type HTTPServer struct {
	*httptest.Server
}

// NewHTTPServer 启动 httptest.Server
//
// 用法：
//
//	srv := testutil.NewHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
//	    fmt.Fprintln(w, "ok")
//	})
//	defer srv.Close()
//	resp, _ := http.Get(srv.URL)
func NewHTTPServer(t *testing.T, handler http.Handler) *HTTPServer {
	t.Helper()
	return &HTTPServer{Server: httptest.NewServer(handler)}
}

// WaitReady 等待端口可访问
func (s *HTTPServer) WaitReady(timeout time.Duration) error {
	return WaitUntil(timeout, 50*time.Millisecond, func() bool {
		resp, err := http.Get(s.URL)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	})
}

// —— Mock Registry ——

// MockRegistry 测试用注册中心（实现 Registrar + Discovery + Watcher）
//
// 用法：
//
//	reg := testutil.NewMockRegistry()
//	_ = reg.Register(ctx, instance)
//	entry, _ := reg.GetService(ctx, "svc")
type MockRegistry struct {
	mu        sync.RWMutex
	instances map[string][]*types.Instance
	notifies  map[string]chan struct{}
}

// NewMockRegistry 创建空 mock
func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		instances: make(map[string][]*types.Instance),
		notifies:  make(map[string]chan struct{}),
	}
}

// Register 注册实例
func (m *MockRegistry) Register(ctx context.Context, ins *types.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instances[ins.Name] = append(m.instances[ins.Name], ins)
	m.notify(ins.Name)
	return nil
}

// Deregister 反注册
func (m *MockRegistry) Deregister(ctx context.Context, ins *types.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.instances[ins.Name]
	for i, x := range list {
		if x.ID == ins.ID {
			m.instances[ins.Name] = append(list[:i], list[i+1:]...)
			break
		}
	}
	m.notify(ins.Name)
	return nil
}

// GetService 实现 Discovery.GetService，返回 ServiceEntry 快照
func (m *MockRegistry) GetService(ctx context.Context, name string) (*types.ServiceEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry := types.NewServiceEntry()
	for _, ins := range m.instances[name] {
		_ = entry.AddInstance(ins)
	}
	return entry, nil
}

// Watch 实现 Watcher.Watch，注册一个变更通道
//
// 简化行为：注册/反注册时 close 通道（非阻塞，单次触发）
func (m *MockRegistry) Watch(ctx context.Context, name string) (<-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.notifies[name]
	if !ok {
		ch = make(chan struct{}, 1)
		m.notifies[name] = ch
	}
	return ch, nil
}

// Count 当前注册实例数（用于断言）
func (m *MockRegistry) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, list := range m.instances {
		n += len(list)
	}
	return n
}

// CountByName 按 name 取实例数
func (m *MockRegistry) CountByName(name string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.instances[name])
}

// notify 触发 watcher 通道（非阻塞，单次触发）
func (m *MockRegistry) notify(name string) {
	if ch, ok := m.notifies[name]; ok {
		select {
		case ch <- struct{}{}:
		default:
			// 已有待处理事件，丢弃新的（避免阻塞注册流程）
		}
	}
}

// 编译期断言：MockRegistry 实现了 Registrar + Discovery + Watcher
var (
	_ registry.Registrar = (*MockRegistry)(nil)
	_ registry.Discovery = (*MockRegistry)(nil)
	_ registry.Watcher   = (*MockRegistry)(nil)
)

// —— Mock Balancer ——

// MockBalancer 固定返回预设实例
type MockBalancer struct {
	mu         sync.Mutex
	instance   *types.Instance
	calls      int
	candidates []*types.Instance
}

// NewMockBalancer 创建固定 mock balancer
func NewMockBalancer(ins *types.Instance) *MockBalancer {
	return &MockBalancer{instance: ins}
}

// Next 实现 balancer.Balancer.Next，固定返回预设实例
func (m *MockBalancer) Next() (*types.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.instance != nil {
		return m.instance, nil
	}
	if len(m.candidates) > 0 {
		return m.candidates[0], nil
	}
	return nil, errors.New("no candidates")
}

// Reload 实现 balancer.Balancer.Reload，记录候选列表
func (m *MockBalancer) Reload(candidates []*types.Instance) balancer.Balancer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.candidates = candidates
	return m
}

// Calls 返回 Next() 调用次数（用于断言）
func (m *MockBalancer) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// 编译期断言
var _ balancer.Balancer = (*MockBalancer)(nil)

// —— 断言辅助 ——

// AssertPanic 断言 fn 会 panic
func AssertPanic(t *testing.T, msg string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic %q but got none", msg)
		}
		s := fmt.Sprintf("%v", r)
		if msg != "" && !contains(s, msg) {
			t.Errorf("panic msg = %q, want substring %q", s, msg)
		}
	}()
	fn()
}

// RequireNoError 失败立即终止
func RequireNoError(t *testing.T, err error, msg ...string) {
	t.Helper()
	if err != nil {
		prefix := ""
		if len(msg) > 0 {
			prefix = msg[0] + ": "
		}
		t.Fatalf("%s%v", prefix, err)
	}
}

// AssertEqual 简单等值断言（避免引入 testify）
func AssertEqual(t *testing.T, want, got any, msg ...string) {
	t.Helper()
	if !equal(want, got) {
		prefix := ""
		if len(msg) > 0 {
			prefix = msg[0] + ": "
		}
		t.Errorf("%sexpect %v, got %v", prefix, want, got)
	}
}

// AssertNotEqual 不等断言
func AssertNotEqual(t *testing.T, a, b any, msg ...string) {
	t.Helper()
	if equal(a, b) {
		prefix := ""
		if len(msg) > 0 {
			prefix = msg[0] + ": "
		}
		t.Errorf("%sshould not equal: %v", prefix, a)
	}
}

// —— 内部 ——

func equal(a, b any) bool {
	return a == b
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
