package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

// mockBalancer 模拟负载均衡器
type mockBalancer struct {
	mu        sync.Mutex
	instances []*types.Instance
}

func (m *mockBalancer) Next() (*types.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.instances) == 0 {
		return nil, http.ErrNoCookie
	}
	return m.instances[0], nil
}

func (m *mockBalancer) Reload(ins []*types.Instance) balancer.Balancer {
	return &mockBalancer{instances: ins}
}

// mockDiscovery 模拟服务发现，同时实现 Discovery 和 Watcher 接口
type mockDiscovery struct {
	mu  sync.RWMutex
	srv *types.ServiceEntry
	ch  chan struct{}
}

func (m *mockDiscovery) GetService(_ context.Context, _ string) (*types.ServiceEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.srv, nil
}

func (m *mockDiscovery) Watch(_ context.Context, _ string) (<-chan struct{}, error) {
	return m.ch, nil
}

// newTestClient 创建用于测试的 client（不启动 watcher goroutine）
func newTestClient(opts ...Option) *client {
	c := &client{
		name:       "test-svc",
		cc:         &http.Client{},
		ownsClient: true,
		clusters:   make(map[string]balancer.Balancer),
		stopCh:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.applyTransportSettings()
	if c.timeout > 0 {
		c.cc.Timeout = c.timeout
	}
	return c
}

func TestResolveCluster_Header(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com", nil)
	r.Header.Set(routing.HeaderCluster, "canary")
	cluster := resolveCluster(r)
	if cluster != "canary" {
		t.Fatalf("expected canary, got %q", cluster)
	}
}

func TestResolveCluster_Context(t *testing.T) {
	ctx := routing.WithCluster(context.Background(), "gray")
	r := httptest.NewRequest("GET", "http://example.com", nil).WithContext(ctx)
	cluster := resolveCluster(r)
	if cluster != "gray" {
		t.Fatalf("expected gray, got %q", cluster)
	}
}

func TestResolveCluster_HeaderPriority(t *testing.T) {
	// Header 优先于 context
	ctx := routing.WithCluster(context.Background(), "gray")
	r := httptest.NewRequest("GET", "http://example.com", nil).WithContext(ctx)
	r.Header.Set(routing.HeaderCluster, "canary")
	cluster := resolveCluster(r)
	if cluster != "canary" {
		t.Fatalf("expected canary (header should win), got %q", cluster)
	}
}

func TestResolveCluster_Default(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com", nil)
	cluster := resolveCluster(r)
	if cluster != routing.Default {
		t.Fatalf("expected default, got %q", cluster)
	}
}

func TestDo_FallbackToDefault(t *testing.T) {
	c := &client{
		name:       "test",
		clusters:   make(map[string]balancer.Balancer),
		clustersMu: sync.RWMutex{},
		cc:         http.DefaultClient,
		stopCh:     make(chan struct{}),
	}

	// 只注册 default 集群，请求带 canary cluster
	c.clustersMu.Lock()
	c.clusters[routing.Default] = &mockBalancer{
		instances: []*types.Instance{{IP: "127.0.0.1", Port: 1234}},
	}
	c.clustersMu.Unlock()

	r := httptest.NewRequest("GET", "http://example.com", nil)
	r.Header.Set(routing.HeaderCluster, "canary")

	// 虽然请求了 canary，但只有 default 集群，应 fallback
	c.clustersMu.RLock()
	lb, ok := c.clusters["canary"]
	if !ok {
		lb, ok = c.clusters[routing.Default]
	}
	c.clustersMu.RUnlock()

	if !ok {
		t.Fatal("expected fallback to default cluster")
	}
	ins, err := lb.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ins.IP != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1, got %s", ins.IP)
	}
}

// TestNewClient 无选项创建客户端
func TestNewClient(t *testing.T) {
	c := NewClient("test-svc")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	// 关闭客户端，清理 watcher goroutine
	c.(*client).Close()
}

// TestNewClient_WithOptions 使用 Discovery 和 LoadBalance 选项创建客户端
func TestNewClient_WithOptions(t *testing.T) {
	dis := &mockDiscovery{
		srv: types.NewServiceEntry(),
		ch:  make(chan struct{}),
	}
	lb := &mockBalancer{}

	c := NewClient("test-svc", Discovery(dis), LoadBalance(lb))
	if c == nil {
		t.Fatal("expected non-nil client")
	}

	inner := c.(*client)
	if inner.dis == nil {
		t.Fatal("expected dis to be set via Discovery option")
	}
	if inner.lb == nil {
		t.Fatal("expected lb to be set via LoadBalance option")
	}

	inner.Close()
}

// TestClose_DoubleClose 多次调用 Close 不应 panic
func TestClose_DoubleClose(t *testing.T) {
	c := newTestClient()
	// 第一次关闭
	c.Close()
	// 第二次关闭不应 panic
	c.Close()
}

// TestDo_InvalidRequest 传入无效请求应返回错误
func TestDo_InvalidRequest(t *testing.T) {
	c := newTestClient()

	// nil request
	_, err := c.Do(nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}

	// nil URL
	r, _ := http.NewRequest("GET", "", nil)
	r.URL = nil
	_, err = c.Do(r)
	if err == nil {
		t.Fatal("expected error for nil URL")
	}
}

// TestDo_NoCluster 没有可用集群时应返回错误
func TestDo_NoCluster(t *testing.T) {
	c := newTestClient()

	r := httptest.NewRequest("GET", "http://example.com/api", nil)
	_, err := c.Do(r)
	if err == nil {
		t.Fatal("expected error when no clusters available")
	}
}

// TestDo_Success 设置真实 HTTP 服务器，验证请求能到达
func TestDo_Success(t *testing.T) {
	// 启动测试 HTTP 服务器
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello from %s", r.URL.Path)
	}))
	defer srv.Close()

	// 解析测试服务器地址
	srvURL, err := parseHostPort(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	c := newTestClient()
	// 注册 default 集群，实例指向测试服务器
	c.clustersMu.Lock()
	c.clusters[routing.Default] = &mockBalancer{
		instances: []*types.Instance{{
			IP:   srvURL.ip,
			Port: srvURL.port,
		}},
	}
	c.clustersMu.Unlock()

	// 使用 http.NewRequest 而非 httptest.NewRequest，
	// 因为 http.Client.Do 不允许 Request.RequestURI 被设置
	r, err := http.NewRequest("GET", "http://example.com/api/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestDo_QueryPreserved 验证 Do 方法保留请求的 Query 参数
// 这是生产环境关键路径：原来 URL 重写只保留 Path，丢失 RawQuery
func TestDo_QueryPreserved(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, err := parseHostPort(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	c := newTestClient()
	c.clustersMu.Lock()
	c.clusters[routing.Default] = &mockBalancer{
		instances: []*types.Instance{{IP: srvURL.ip, Port: srvURL.port}},
	}
	c.clustersMu.Unlock()

	r, err := http.NewRequest("GET", "http://example.com/api/data?foo=bar&page=42", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if receivedQuery != "foo=bar&page=42" {
		t.Fatalf("query should be preserved, got %q", receivedQuery)
	}
}

// TestDo_FallbackToDefault_ThroughDo 通过 Do 方法验证集群回退到 default 集群
func TestDo_FallbackToDefault_ThroughDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, err := parseHostPort(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	c := newTestClient()
	// 只注册 default 集群
	c.clustersMu.Lock()
	c.clusters[routing.Default] = &mockBalancer{
		instances: []*types.Instance{{
			IP:   srvURL.ip,
			Port: srvURL.port,
		}},
	}
	c.clustersMu.Unlock()

	// 请求带 canary cluster，但只有 default 集群
	r, err := http.NewRequest("GET", "http://example.com/api", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	r.Header.Set(routing.HeaderCluster, "canary")

	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("expected fallback to default, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestLoad_NilDiscovery dis 为 nil 时 load 是空操作
func TestLoad_NilDiscovery(t *testing.T) {
	c := newTestClient()
	// dis 为 nil，load 不应 panic
	c.load()
	// clusters 应保持为空
	c.clustersMu.RLock()
	n := len(c.clusters)
	c.clustersMu.RUnlock()
	if n != 0 {
		t.Fatalf("expected 0 clusters, got %d", n)
	}
}

// TestDiscoveryOption 验证 Discovery 选项设置 c.dis
func TestDiscoveryOption(t *testing.T) {
	dis := &mockDiscovery{
		srv: types.NewServiceEntry(),
		ch:  make(chan struct{}),
	}

	c := newTestClient()
	Discovery(dis)(c)

	if c.dis == nil {
		t.Fatal("expected dis to be set")
	}
}

// TestLoadBalanceOption 验证 LoadBalance 选项设置 c.lb
func TestLoadBalanceOption(t *testing.T) {
	lb := &mockBalancer{}

	c := newTestClient()
	LoadBalance(lb)(c)

	if c.lb == nil {
		t.Fatal("expected lb to be set")
	}
}

// TestDo_Concurrent 并发调用 Do，验证无数据竞争
func TestDo_Concurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, err := parseHostPort(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	c := newTestClient()
	c.clustersMu.Lock()
	c.clusters[routing.Default] = &mockBalancer{
		instances: []*types.Instance{{
			IP:   srvURL.ip,
			Port: srvURL.port,
		}},
	}
	c.clustersMu.Unlock()

	var wg sync.WaitGroup
	const n = 20
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := http.NewRequest("GET", "http://example.com/api", nil)
			if err != nil {
				errCh <- err
				return
			}
			resp, err := c.Do(r)
			if err != nil {
				errCh <- err
				return
			}
			resp.Body.Close()
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent Do failed: %v", err)
	}
}

// TestDo_ClusterPropagatedFromContextToHeader 验证 Do 把 ctx 中的 cluster 写回 header
// 否则 cluster 仅存于 ctx，HTTP 转发后下游 server 读到 default
func TestDo_ClusterPropagatedFromContextToHeader(t *testing.T) {
	var receivedCluster string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟 server/http 的 clusterInjector：从 header 读 cluster
		receivedCluster = r.Header.Get(routing.HeaderCluster)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, err := parseHostPort(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	c := newTestClient()
	// 注册 canary 集群实例
	c.clustersMu.Lock()
	c.clusters["canary"] = &mockBalancer{
		instances: []*types.Instance{{IP: srvURL.ip, Port: srvURL.port}},
	}
	c.clustersMu.Unlock()

	// 创建请求，ctx 中带 cluster=canary，但 header 不显式设置
	r, err := http.NewRequest("GET", "http://example.com/api", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	ctx := routing.WithCluster(context.Background(), "canary")
	r = r.WithContext(ctx)

	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if receivedCluster != "canary" {
		t.Fatalf("downstream should receive X-Zeus-Cluster=canary header, got %q", receivedCluster)
	}
}

// TestDo_ClusterHeaderPreservedIfExplicit 验证显式设置的 cluster header 不被覆盖
func TestDo_ClusterHeaderPreservedIfExplicit(t *testing.T) {
	var receivedCluster string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCluster = r.Header.Get(routing.HeaderCluster)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, _ := parseHostPort(srv.URL)

	c := newTestClient()
	c.clustersMu.Lock()
	c.clusters["canary"] = &mockBalancer{
		instances: []*types.Instance{{IP: srvURL.ip, Port: srvURL.port}},
	}
	c.clustersMu.Unlock()

	r, _ := http.NewRequest("GET", "http://example.com/api", nil)
	r.Header.Set(routing.HeaderCluster, "canary") // 显式设置
	// ctx 中是别的值（应该被忽略，header 优先）
	ctx := routing.WithCluster(context.Background(), "default")
	r = r.WithContext(ctx)

	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if receivedCluster != "canary" {
		t.Fatalf("explicit header cluster should be preserved, got %q", receivedCluster)
	}
}

// hostPort 用于解析 httptest.Server 的地址
type hostPort struct {
	ip   string
	port int
}

// parseHostPort 从 URL 解析出 ip 和 port
func parseHostPort(rawURL string) (hostPort, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return hostPort{}, err
	}
	host := u.Hostname()
	port := 80
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	return hostPort{ip: host, port: port}, nil
}

// TestDo_PropagationAutoInject 用户在 ctx 写入自定义 K-V 后，client.Do 自动注入 baggage header
//
// 这是 propagation 集成 client 的核心契约：
//   - 用户调用 propagation.With(ctx, "tenant.id", "acme")
//   - 调 client.Do(req.WithContext(ctx))
//   - server 端收到 Baggage header，自动 extract 到 ctx
func TestDo_PropagationAutoInject(t *testing.T) {
	var receivedBaggage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBaggage = r.Header.Get(propagation.HeaderBaggage)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, _ := parseHostPort(srv.URL)

	c := newTestClient()
	c.clustersMu.Lock()
	c.clusters["default"] = &mockBalancer{
		instances: []*types.Instance{{IP: srvURL.ip, Port: srvURL.port}},
	}
	c.clustersMu.Unlock()

	// 用户在业务代码注入自定义 K-V
	ctx := propagation.With(context.Background(), "tenant.id", "acme-corp")
	ctx = propagation.With(ctx, "feature.flag", "beta")

	r, _ := http.NewRequest("GET", "http://example.com/api", nil)
	r = r.WithContext(ctx)

	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if receivedBaggage == "" {
		t.Fatal("Baggage header should be auto-injected by client.Do")
	}
	// 验证两个 K-V 都被透传
	bag := propagation.Decode(receivedBaggage)
	if v, _ := bag.Get("tenant.id"); v != "acme-corp" {
		t.Errorf("tenant.id = %q, want acme-corp", v)
	}
	if v, _ := bag.Get("feature.flag"); v != "beta" {
		t.Errorf("feature.flag = %q, want beta", v)
	}
}

// TestDo_PropagationClusterIncluded 当用户用 routing.WithCluster 时，
// baggage header 中也包含 zeus.cluster（双向同步的体现）
func TestDo_PropagationClusterIncluded(t *testing.T) {
	var receivedBaggage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBaggage = r.Header.Get(propagation.HeaderBaggage)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, _ := parseHostPort(srv.URL)

	c := newTestClient()
	c.clustersMu.Lock()
	c.clusters["canary"] = &mockBalancer{
		instances: []*types.Instance{{IP: srvURL.ip, Port: srvURL.port}},
	}
	c.clustersMu.Unlock()

	// routing.WithCluster 同时写入 ctx 和 propagation Bag
	ctx := routing.WithCluster(context.Background(), "canary")
	r, _ := http.NewRequest("GET", "http://example.com/api", nil)
	r = r.WithContext(ctx)

	resp, err := c.Do(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	bag := propagation.Decode(receivedBaggage)
	if v, _ := bag.Get(routing.BagKey); v != "canary" {
		t.Errorf("baggage[zeus.cluster] = %q, want canary", v)
	}
}
