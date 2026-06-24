package components

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/server"
	"github.com/go-zeus/zeus/types"
)

// mockRegistrar 用于测试的 mock 注册中心，记录 Register/Deregister 调用
type mockRegistrar struct {
	registered   []*types.Instance
	deregistered []*types.Instance
	registerErr  error
	deregErr     error
}

func (m *mockRegistrar) Register(_ context.Context, ins *types.Instance) error {
	if m.registerErr != nil {
		return m.registerErr
	}
	m.registered = append(m.registered, ins)
	return nil
}

func (m *mockRegistrar) Deregister(_ context.Context, ins *types.Instance) error {
	if m.deregErr != nil {
		return m.deregErr
	}
	m.deregistered = append(m.deregistered, ins)
	return nil
}

// 编译期检查 mockRegistrar 实现 Registrar 接口
var _ registry.Registrar = (*mockRegistrar)(nil)

// mockServer 测试用的 server，固定 Endpoint/Protocol
type mockServer struct {
	endpoint string
	protocol string
}

func (m *mockServer) Protocol() string              { return m.protocol }
func (m *mockServer) Endpoint() string              { return m.endpoint }
func (m *mockServer) Start(_ context.Context) error { return nil }
func (m *mockServer) Stop(_ context.Context) error  { return nil }

// 编译期检查 mockServer 实现 server.Server 接口
var _ server.Server = (*mockServer)(nil)

// newTestServiceComponent 构造一个用于测试的 ServiceComponent，并完成 Provide
func newTestServiceComponent(t *testing.T, servers []server.Server, opts ...ServiceOption) *ServiceComponent {
	t.Helper()
	sc := NewServiceComponent(opts...)
	ctx := newAssemblyContext()
	ctx.set("server", servers)
	if _, err := sc.Provide(ctx); err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	return sc
}

// TestServiceComponent_OnStart_Registers 验证 OnStart 调用 Registrar.Register（单 server）
func TestServiceComponent_OnStart_Registers(t *testing.T) {
	mockReg := &mockRegistrar{}
	servers := []server.Server{&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"}}
	sc := newTestServiceComponent(t, servers,
		WithServiceName("test-svc"),
		WithServiceIP("127.0.0.1"),
	)

	ctx := newAssemblyContext()
	ctx.set("registry", mockReg)

	if err := sc.Lifecycle().OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}

	if len(mockReg.registered) != 1 {
		t.Fatalf("expected 1 registered instance, got %d", len(mockReg.registered))
	}
	ins := mockReg.registered[0]
	if ins.Name != "test-svc" {
		t.Errorf("registered name = %q, want test-svc", ins.Name)
	}
	if ins.Protocol != "http" {
		t.Errorf("registered protocol = %q, want http", ins.Protocol)
	}
	if ins.Port != 9001 {
		t.Errorf("registered port = %d, want 9001", ins.Port)
	}
}

// TestServiceComponent_OnStart_RegistersMultipleServers 验证多 server 场景下注册多个 Instance
func TestServiceComponent_OnStart_RegistersMultipleServers(t *testing.T) {
	mockReg := &mockRegistrar{}
	servers := []server.Server{
		&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"},
		&mockServer{endpoint: "127.0.0.1:9002", protocol: "grpc"},
	}
	sc := newTestServiceComponent(t, servers, WithServiceName("multi-svc"))

	ctx := newAssemblyContext()
	ctx.set("registry", mockReg)

	if err := sc.Lifecycle().OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}

	if got, want := len(mockReg.registered), 2; got != want {
		t.Fatalf("expected %d registered instances, got %d", want, got)
	}
	// 每个 Instance 应有不同 protocol 和 port
	byProto := map[string]*types.Instance{}
	for _, ins := range mockReg.registered {
		byProto[ins.Protocol] = ins
	}
	if ins, ok := byProto["http"]; !ok || ins.Port != 9001 {
		t.Errorf("http instance missing or wrong port: %+v", ins)
	}
	if ins, ok := byProto["grpc"]; !ok || ins.Port != 9002 {
		t.Errorf("grpc instance missing or wrong port: %+v", ins)
	}
	// 两个 Instance 应共享同一 Name
	if byProto["http"].Name != byProto["grpc"].Name {
		t.Errorf("instances should share Name, got %q vs %q",
			byProto["http"].Name, byProto["grpc"].Name)
	}
}

// TestServiceComponent_OnStart_RegisterError 验证 OnStart 注册失败时返回错误
func TestServiceComponent_OnStart_RegisterError(t *testing.T) {
	mockReg := &mockRegistrar{
		registerErr: errors.New("etcd unavailable"),
	}
	sc := newTestServiceComponent(t,
		[]server.Server{&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"}},
		WithServiceName("test-svc"),
	)

	ctx := newAssemblyContext()
	ctx.set("registry", mockReg)

	err := sc.Lifecycle().OnStart(ctx)
	if err == nil {
		t.Fatal("expected Register error to propagate")
	}
	if !strings.Contains(err.Error(), "register instance") {
		t.Errorf("error should mention register instance, got: %v", err)
	}
}

// TestServiceComponent_OnStart_NoRegistry_NoOp 验证无 registry 组件时跳过注册不报错
func TestServiceComponent_OnStart_NoRegistry_NoOp(t *testing.T) {
	sc := newTestServiceComponent(t,
		[]server.Server{&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"}},
		WithServiceName("test-svc"),
	)

	// 故意不注册 "registry" 组件
	ctx := newAssemblyContext()

	if err := sc.Lifecycle().OnStart(ctx); err != nil {
		t.Fatalf("OnStart without registry should not fail, got: %v", err)
	}
}

// TestServiceComponent_OnStop_Deregisters 验证 OnStop 调用 Registrar.Deregister
func TestServiceComponent_OnStop_Deregisters(t *testing.T) {
	mockReg := &mockRegistrar{}
	servers := []server.Server{&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"}}
	sc := newTestServiceComponent(t, servers, WithServiceName("test-svc"))

	ctx := newAssemblyContext()
	ctx.set("registry", mockReg)

	lc := sc.Lifecycle()
	// 先注册
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	// 再反注册
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}

	if len(mockReg.deregistered) != 1 {
		t.Fatalf("expected 1 deregistered instance, got %d", len(mockReg.deregistered))
	}
	if mockReg.deregistered[0].Name != "test-svc" {
		t.Errorf("deregistered name = %q, want test-svc", mockReg.deregistered[0].Name)
	}
}

// TestServiceComponent_OnStop_DeregisterError 验证 OnStop 反注册失败不阻塞关闭流程
func TestServiceComponent_OnStop_DeregisterError(t *testing.T) {
	mockReg := &mockRegistrar{
		deregErr: errors.New("etcd unavailable"),
	}
	sc := newTestServiceComponent(t,
		[]server.Server{&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"}},
		WithServiceName("test-svc"),
	)

	ctx := newAssemblyContext()
	ctx.set("registry", mockReg)

	// OnStop 即使 Deregister 失败也应返回 nil（不阻塞其他组件关闭）
	if err := sc.Lifecycle().OnStop(ctx); err != nil {
		t.Fatalf("OnStop should swallow Deregister error, got: %v", err)
	}
}

// TestServiceComponent_OnStop_NoRegistry_NoOp 验证无 registry 时 OnStop 不报错
func TestServiceComponent_OnStop_NoRegistry_NoOp(t *testing.T) {
	sc := newTestServiceComponent(t,
		[]server.Server{&mockServer{endpoint: "127.0.0.1:9001", protocol: "http"}},
		WithServiceName("test-svc"),
	)

	ctx := newAssemblyContext()
	// 不设置 registry

	if err := sc.Lifecycle().OnStop(ctx); err != nil {
		t.Fatalf("OnStop without registry should not fail, got: %v", err)
	}
}

// TestServiceComponent_OnStart_OnStop_NilInstances 验证未 Provide（instances 为 nil）时不 panic
func TestServiceComponent_OnStart_OnStop_NilInstances(t *testing.T) {
	sc := &ServiceComponent{} // instances 为 nil
	ctx := newAssemblyContext()
	ctx.set("registry", &mockRegistrar{})

	if err := sc.Lifecycle().OnStart(ctx); err != nil {
		t.Fatalf("OnStart with nil instances should not fail, got: %v", err)
	}
	if err := sc.Lifecycle().OnStop(ctx); err != nil {
		t.Fatalf("OnStop with nil instances should not fail, got: %v", err)
	}
}

// TestParseEndpoint 验证 endpoint 解析
func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		endpoint string
		wantHost string
		wantPort int
	}{
		{"127.0.0.1:8080", "127.0.0.1", 8080},
		{":8080", "", 8080},
		{"10.0.0.1:9001", "10.0.0.1", 9001},
		{"no-port", "no-port", 0},
		// IPv6 必须使用 net.SplitHostPort 解析，否则会拆出 "0" 作为端口
		{"[::1]:8080", "::1", 8080},
		{"[2001:db8::1]:9001", "2001:db8::1", 9001},
	}
	for _, c := range cases {
		gotHost, gotPort := parseEndpoint(c.endpoint)
		if gotHost != c.wantHost || gotPort != c.wantPort {
			t.Errorf("parseEndpoint(%q) = (%q, %d), want (%q, %d)",
				c.endpoint, gotHost, gotPort, c.wantHost, c.wantPort)
		}
	}
}

// TestNewServiceComponent_Defaults 验证默认配置
func TestNewServiceComponent_Defaults(t *testing.T) {
	sc := NewServiceComponent()
	if sc.opts.Name != defaultServiceName {
		t.Errorf("default Name = %q, want %q", sc.opts.Name, defaultServiceName)
	}
	if sc.opts.Cluster != defaultClusterName {
		t.Errorf("default Cluster = %q, want %q", sc.opts.Cluster, defaultClusterName)
	}
	if sc.opts.IP == "" {
		t.Error("default Ip should be auto-detected (non-empty)")
	}
}

// TestNewServiceComponent_OptionsApplied 验证 Options 生效
func TestNewServiceComponent_OptionsApplied(t *testing.T) {
	sc := NewServiceComponent(
		WithServiceName("my-app"),
		WithServiceCluster("canary"),
		WithServiceIP("10.0.0.1"),
	)
	if sc.opts.Name != "my-app" {
		t.Errorf("Name = %q, want my-app", sc.opts.Name)
	}
	if sc.opts.Cluster != "canary" {
		t.Errorf("Cluster = %q, want canary", sc.opts.Cluster)
	}
	if sc.opts.IP != "10.0.0.1" {
		t.Errorf("Ip = %q, want 10.0.0.1", sc.opts.IP)
	}
}
