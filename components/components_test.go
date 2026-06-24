package components

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/circuitbreaker/counter"
	"github.com/go-zeus/zeus/config/file"
	logslog "github.com/go-zeus/zeus/log/slog"
	metricsnoop "github.com/go-zeus/zeus/metrics/noop"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/ratelimit"
	"github.com/go-zeus/zeus/ratelimit/token"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/retry"
	"github.com/go-zeus/zeus/retry/exponential"
	zeushttp "github.com/go-zeus/zeus/server/http"
	tracenoop "github.com/go-zeus/zeus/trace/noop"
	"github.com/go-zeus/zeus/types"
)

// freePort 获取一个系统当前空闲的端口（TOCTOU 不可避免，但 95% 场景下足够）。
// 测试避免硬编码 8080 等常用端口，与开发环境冲突。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// mockComponent 用于测试的模拟组件
type mockComponent struct {
	name     string
	depends  []string
	provide  any
	startErr error
	stopErr  error
	started  bool
	stopped  bool
}

func (m *mockComponent) Name() string      { return m.name }
func (m *mockComponent) Depends() []string { return m.depends }
func (m *mockComponent) Provide(ctx Context) (any, error) {
	return m.provide, nil
}
func (m *mockComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			m.started = true
			return m.startErr
		},
		OnStop: func(ctx Context) error {
			m.stopped = true
			return m.stopErr
		},
	}
}

func TestNewContainer(t *testing.T) {
	c := NewContainer()
	if c == nil {
		t.Fatal("NewContainer returned nil")
	}
}

func TestRegister(t *testing.T) {
	c := NewContainer()
	err := c.Register(&mockComponent{name: "a", provide: "value_a"})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", provide: 1})
	err := c.Register(&mockComponent{name: "a", provide: 2})
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegisterEmptyName(t *testing.T) {
	c := NewContainer()
	err := c.Register(&mockComponent{name: "", provide: 1})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestResolve_Simple(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", provide: 1})
	_ = c.Register(&mockComponent{name: "b", depends: []string{"a"}, provide: 2})
	err := c.Resolve()
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	// a 应在 b 之前
	if c.order[0] != "a" || c.order[1] != "b" {
		t.Fatalf("expected order [a, b], got %v", c.order)
	}
}

func TestResolve_CircularDependency(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", depends: []string{"b"}, provide: 1})
	_ = c.Register(&mockComponent{name: "b", depends: []string{"a"}, provide: 2})
	err := c.Resolve()
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestResolve_MissingDependency(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", depends: []string{"missing"}, provide: 1})
	err := c.Resolve()
	if err == nil {
		t.Fatal("expected error for missing dependency")
	}
}

func TestStartStop(t *testing.T) {
	c := NewContainer()
	a := &mockComponent{name: "a", provide: "value_a"}
	b := &mockComponent{name: "b", depends: []string{"a"}, provide: "value_b"}
	_ = c.Register(a)
	_ = c.Register(b)

	err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !a.started || !b.started {
		t.Fatal("expected both components to be started")
	}

	// 验证实例可获取
	v, ok := c.Get("a")
	if !ok || v != "value_a" {
		t.Fatalf("expected value_a, got %v, ok=%v", v, ok)
	}
	v, ok = c.Get("b")
	if !ok || v != "value_b" {
		t.Fatalf("expected value_b, got %v, ok=%v", v, ok)
	}

	// 停止
	err = c.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	// b 应先于 a 停止（逆序）
	if !a.stopped || !b.stopped {
		t.Fatal("expected both components to be stopped")
	}
}

func TestStart_ProvideDependency(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "config", provide: map[string]string{"key": "val"}})

	// server 依赖 config，通过 Context 获取
	serverStarted := false
	_ = c.Register(&mockCompWithCtx{
		name:    "server",
		depends: []string{"config"},
		onStart: func(ctx Context) error {
			v, err := ctx.Get("config")
			if err != nil {
				return fmt.Errorf("config not found: %w", err)
			}
			m, ok := v.(map[string]string)
			if !ok || m["key"] != "val" {
				return fmt.Errorf("unexpected config value")
			}
			serverStarted = true
			return nil
		},
	})

	err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !serverStarted {
		t.Fatal("server should have started with config dependency")
	}
}

func TestStart_FailureRollback(t *testing.T) {
	c := NewContainer()
	a := &mockComponent{name: "a", provide: 1}
	b := &mockComponent{name: "b", depends: []string{"a"}, provide: 2, startErr: fmt.Errorf("boom")}
	_ = c.Register(a)
	_ = c.Register(b)

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when component fails to start")
	}

	// a 已启动，应被逆序停止
	if !a.started {
		t.Fatal("a should have started")
	}
	if !a.stopped {
		t.Fatal("a should have been stopped during rollback")
	}
}

func TestGetByType(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "config", provide: map[string]string{"k": "v"}})
	_ = c.Start(context.Background())

	m, ok := GetByType[map[string]string](c)
	if !ok {
		t.Fatal("GetByType should find map[string]string")
	}
	if m["k"] != "v" {
		t.Fatalf("expected k=v, got %v", m)
	}
}

// TestGetType_InterfaceMatch 验证按接口类型查找时能正确匹配实现
// 通过 Type[T](ctx) 顶层泛型函数获取（Go 接口不支持泛型方法）
func TestGetType_InterfaceMatch(t *testing.T) {
	ctx := newAssemblyContext()
	ctx.set("registry", &mockComponent{name: "registry"})

	// 按 mockComponent 接口类型查找应能命中
	_, err := Type[*mockComponent](ctx)
	if err != nil {
		t.Fatalf("Type[*mockComponent] failed: %v", err)
	}

	// 不存在的类型应返回 error
	_, err = Type[*mockRegistrar](ctx)
	if err == nil {
		t.Fatal("GetType for unregistered type should return error")
	}
}

// TestContext_EmbedsStdlibContext 验证 Context 嵌入 context.Context
// 生命周期钩子可感知容器注入的 stdlib context（取消/超时/值传递）
func TestContext_EmbedsStdlibContext(t *testing.T) {
	type ctxKey struct{}
	c := NewContainer()

	var seenCtx context.Context
	_ = c.Register(&mockCompWithCtx{
		name:    "ctxprobe",
		depends: nil,
		onStart: func(ctx Context) error {
			seenCtx = ctx
			// 通过嵌入的 stdlib context 读取用户值
			if v := ctx.Value(ctxKey{}); v != "hello" {
				t.Errorf("ctx.Value = %v, want hello", v)
			}
			return nil
		},
	})

	// 注入携带值的 stdlib context
	ctx := context.WithValue(context.Background(), ctxKey{}, "hello")
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if seenCtx == nil {
		t.Fatal("OnStart not invoked")
	}
	// 验证 Done/Err 等 stdlib API 可用
	if err := seenCtx.Err(); err != nil {
		t.Errorf("seenCtx.Err = %v, want nil", err)
	}
	_ = c.Stop(context.Background())
}

func TestStartOrder_Complex(t *testing.T) {
	// 测试复杂依赖拓扑: d → b → a, d → c → a
	c := NewContainer()
	var order []string
	register := func(name string, deps []string) {
		_ = c.Register(&mockCompWithCtx{
			name:    name,
			depends: deps,
			onStart: func(ctx Context) error {
				order = append(order, name)
				return nil
			},
		})
	}
	register("a", nil)
	register("b", []string{"a"})
	register("c", []string{"a"})
	register("d", []string{"b", "c"})

	err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// a 必须在 b 和 c 之前，b 和 c 必须在 d 之前
	pos := func(name string) int {
		for i, n := range order {
			if n == name {
				return i
			}
		}
		return -1
	}
	if pos("a") >= pos("b") || pos("a") >= pos("c") {
		t.Fatalf("a must come before b and c, got order: %v", order)
	}
	if pos("b") >= pos("d") || pos("c") >= pos("d") {
		t.Fatalf("b and c must come before d, got order: %v", order)
	}
}

// mockCompWithCtx 带自定义 OnStart 的测试组件
type mockCompWithCtx struct {
	name    string
	depends []string
	onStart func(ctx Context) error
	onStop  func(ctx Context) error
}

func (m *mockCompWithCtx) Name() string                     { return m.name }
func (m *mockCompWithCtx) Depends() []string                { return m.depends }
func (m *mockCompWithCtx) Provide(ctx Context) (any, error) { return m.name + "_instance", nil }
func (m *mockCompWithCtx) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: m.onStart,
		OnStop:  m.onStop,
	}
}

func TestStart_ProvideFailureRollback(t *testing.T) {
	// A 的 Provide 成功，B 的 Provide 失败，验证 A 的 OnStop 被调用
	aStopped := false
	c := NewContainer()
	_ = c.Register(&mockCompWithCtx{
		name:    "a",
		onStart: func(ctx Context) error { return nil },
		onStop:  func(ctx Context) error { aStopped = true; return nil },
	})
	_ = c.Register(&provideFailComp{name: "b", depends: []string{"a"}})

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when Provide fails")
	}

	if !aStopped {
		t.Fatal("a's OnStop should have been called during rollback after b's Provide failure")
	}
}

// provideFailComp Provide 总是失败的测试组件
type provideFailComp struct {
	name    string
	depends []string
}

func (p *provideFailComp) Name() string                     { return p.name }
func (p *provideFailComp) Depends() []string                { return p.depends }
func (p *provideFailComp) Provide(ctx Context) (any, error) { return nil, fmt.Errorf("provide failed") }
func (p *provideFailComp) Lifecycle() Lifecycle             { return Lifecycle{} }

func TestStop_ContextCancellation(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", provide: 1})

	err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 使用已取消的 context 调用 Stop
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = c.Stop(ctx)
	if err == nil {
		t.Fatal("expected context error when stopping with cancelled context, got nil")
	}
	if ctx.Err() != err {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
}

func TestConcurrentStartStop(t *testing.T) {
	// 使用多个独立 Container 并发启动和停止，验证无 panic 或死锁
	var wg sync.WaitGroup
	const goroutines = 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := NewContainer()
			_ = c.Register(&mockComponent{name: "a", provide: 1})
			_ = c.Register(&mockComponent{name: "b", depends: []string{"a"}, provide: 2})

			_ = c.Start(context.Background())
			_ = c.Stop(context.Background())
		}()
	}
	wg.Wait()
}

// TestAdapt 测试 Adapt 快速包装组件
func TestAdapt(t *testing.T) {
	comp := Adapt("mydb", "connection_string", "config")

	if comp.Name() != "mydb" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "mydb")
	}
	if len(comp.Depends()) != 1 || comp.Depends()[0] != "config" {
		t.Fatalf("Depends = %v, want [config]", comp.Depends())
	}

	// Provide 应返回包装的值
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val != "connection_string" {
		t.Fatalf("Provide = %v, want %q", val, "connection_string")
	}

	// Lifecycle 应为空（无钩子）
	lc := comp.Lifecycle()
	if lc.OnStart != nil || lc.OnStop != nil {
		t.Fatal("Adapt component should have no lifecycle hooks")
	}

	// 无依赖的 Adapt
	compNoDeps := Adapt("simple", 42)
	if len(compNoDeps.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", compNoDeps.Depends())
	}
}

// TestContainer_DoubleStart 测试重复启动返回错误
func TestContainer_DoubleStart(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", provide: 1})

	err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	err = c.Start(context.Background())
	if err == nil {
		t.Fatal("second Start should return error")
	}
}

// TestContainer_Get_NonExistent 测试获取不存在的组件返回 nil, false
func TestContainer_Get_NonExistent(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", provide: 1})
	_ = c.Start(context.Background())

	val, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("Get with unknown name should return false")
	}
	if val != nil {
		t.Fatalf("Get with unknown name should return nil, got %v", val)
	}
}

// TestRegistryComponent 测试注册中心组件适配器
func TestRegistryComponent(t *testing.T) {
	comp := NewRegistryComponent(memory.New())
	if comp.Name() != "registry" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "registry")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	// RegistryComponent 不需要 OnStart（启动时无操作）
	if lc.OnStart != nil {
		t.Fatal("RegistryComponent should not have OnStart")
	}
	// OnStop 在 reg 实现 io.Closer 时用于关闭连接池
	if lc.OnStop == nil {
		t.Fatal("RegistryComponent should have OnStop (to close Closer)")
	}
	// memory registry 未实现 io.Closer，OnStop 应是 no-op
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop with memory registry (no Closer): %v", err)
	}
}

// TestRegistryComponent_OnStop_ClosesCloser 验证 reg 实现 io.Closer 时 OnStop 调用 Close
type fakeCloserRegistry struct {
	closed bool
}

func (f *fakeCloserRegistry) Register(_ context.Context, _ *types.Instance) error { return nil }
func (f *fakeCloserRegistry) Deregister(_ context.Context, _ *types.Instance) error {
	return nil
}
func (f *fakeCloserRegistry) Close() error {
	f.closed = true
	return nil
}

func TestRegistryComponent_OnStop_ClosesCloser(t *testing.T) {
	fake := &fakeCloserRegistry{}
	comp := NewRegistryComponent(fake)
	lc := comp.Lifecycle()
	if lc.OnStop == nil {
		t.Fatal("OnStop should be set")
	}
	if err := lc.OnStop(newAssemblyContext()); err != nil {
		t.Fatalf("OnStop: %v", err)
	}
	if !fake.closed {
		t.Fatal("OnStop should call Close when reg implements io.Closer")
	}
}

// TestServerComponent 测试服务端组件适配器
func TestServerComponent(t *testing.T) {
	// 用动态端口避免与开发环境 8080 等固定端口冲突
	comp := NewServerComponent(zeushttp.NewHTTP(zeushttp.Port(freePort(t))))
	if comp.Name() != "server" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "server")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	// OnStart 不应报错
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	// OnStop 需要传入有效的 context 来停止 server
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// TestConfigComponent 测试配置组件适配器
func TestConfigComponent(t *testing.T) {
	comp := NewConfigComponent(file.NewFile())
	if comp.Name() != "config" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "config")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	// OnStop 应该正常执行（Close）
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// TestLogComponent 测试日志组件适配器
func TestLogComponent(t *testing.T) {
	comp := NewLogComponent(logslog.NewSlog())
	if comp.Name() != "log" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "log")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	// OnStart 不应报错
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	// OnStop 应该正常执行（Close）
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// TestCircuitBreakerComponent 测试熔断器组件适配器
func TestCircuitBreakerComponent(t *testing.T) {
	comp := NewCircuitBreakerComponent(counter.NewCount())
	if comp.Name() != "circuitbreaker" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "circuitbreaker")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// TestRateLimitComponent 测试限流器组件适配器
func TestRateLimitComponent(t *testing.T) {
	limiter := token.New(100, 10)
	comp := NewRateLimitComponent(limiter)
	if comp.Name() != "ratelimit" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "ratelimit")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
	// 验证返回的是 Limiter 接口
	if _, ok := val.(ratelimit.Limiter); !ok {
		t.Fatal("Provide should return ratelimit.Limiter")
	}
}

// TestRetryComponent 测试重试策略组件适配器
func TestRetryComponent(t *testing.T) {
	ret := exponential.New(3, 100*time.Millisecond)
	comp := NewRetryComponent(ret)
	if comp.Name() != "retry" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "retry")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
	// 验证返回的是 Retrier 接口
	if _, ok := val.(retry.Retrier); !ok {
		t.Fatal("Provide should return retry.Retrier")
	}
}

// TestMiddlewareComponent 测试中间件组件适配器
func TestMiddlewareComponent(t *testing.T) {
	comp := NewMiddlewareComponent(recovery.New())
	// MiddlewareComponent 的 Name 包含驱动名
	if comp.Name() != "middleware_recovery" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "middleware_recovery")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// TestTraceComponent 测试链路追踪组件适配器
func TestTraceComponent(t *testing.T) {
	comp := NewTraceComponent(tracenoop.New())
	if comp.Name() != "trace" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "trace")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	// OnStop 应该正常执行（Close）
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// TestMetricsComponent 测试指标组件适配器
func TestMetricsComponent(t *testing.T) {
	comp := NewMetricsComponent(metricsnoop.New())
	if comp.Name() != "metrics" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "metrics")
	}
	if len(comp.Depends()) != 0 {
		t.Fatalf("Depends = %v, want empty", comp.Depends())
	}
	ctx := newAssemblyContext()
	val, err := comp.Provide(ctx)
	if err != nil {
		t.Fatalf("Provide error: %v", err)
	}
	if val == nil {
		t.Fatal("Provide returned nil")
	}
	lc := comp.Lifecycle()
	if lc.OnStart == nil || lc.OnStop == nil {
		t.Fatal("Lifecycle hooks should not be nil")
	}
	if err := lc.OnStart(ctx); err != nil {
		t.Fatalf("OnStart error: %v", err)
	}
	// OnStop 应该正常执行（Close）
	if err := lc.OnStop(ctx); err != nil {
		t.Fatalf("OnStop error: %v", err)
	}
}

// ---- App 测试 ----

// TestNewApp 测试创建应用
func TestNewApp(t *testing.T) {
	app := NewApp(
		Adapt("a", 1),
		Adapt("b", 2, "a"),
	)
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
}

// TestNewApp_DuplicatePanics 测试重复注册会 panic
func TestNewApp_DuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for duplicate registration")
		}
	}()
	NewApp(
		Adapt("a", 1),
		Adapt("a", 2),
	)
}

// TestApp_Get 测试 App.Get 按名称获取组件
func TestApp_Get(t *testing.T) {
	app := NewApp(Adapt("config", map[string]string{"k": "v"}))
	ctx := context.Background()
	if err := app.container.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	val, ok := app.Get("config")
	if !ok {
		t.Fatal("Get should find config")
	}
	m, ok := val.(map[string]string)
	if !ok || m["k"] != "v" {
		t.Fatalf("unexpected value: %v", val)
	}

	_ = app.container.Stop(ctx)
}

// TestApp_Container 测试获取底层 Container
func TestApp_Container(t *testing.T) {
	app := NewApp(Adapt("a", 1))
	c := app.Container()
	if c == nil {
		t.Fatal("Container should not be nil")
	}
}

// TestApp_RunWithContext 测试 RunWithContext 通过 context 取消
func TestApp_RunWithContext(t *testing.T) {
	startCh := make(chan struct{})
	app := NewApp(&mockCompWithCtx{
		name: "a",
		onStart: func(ctx Context) error {
			close(startCh)
			return nil
		},
		onStop: func(ctx Context) error { return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.RunWithContext(ctx)
	}()

	// 等待组件启动
	select {
	case <-startCh:
	case <-time.After(2 * time.Second):
		t.Fatal("component should have started")
	}

	// 取消 context 触发关闭
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("RunWithContext error: %v", err)
	}
}

// TestGetByTypeFromApp 测试从 App 按类型获取组件
func TestGetByTypeFromApp(t *testing.T) {
	app := NewApp(Adapt("config", map[string]string{"k": "v"}))
	ctx := context.Background()
	_ = app.container.Start(ctx)

	m, ok := GetByTypeFromApp[map[string]string](app)
	if !ok {
		t.Fatal("GetByTypeFromApp should find map[string]string")
	}
	if m["k"] != "v" {
		t.Fatalf("unexpected value: %v", m)
	}

	_ = app.container.Stop(ctx)
}

// TestWithStopTimeout 测试设置关闭超时
func TestWithStopTimeout(t *testing.T) {
	app := NewApp(Adapt("a", 1), WithStopTimeout(5*time.Second))
	if app.timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want 5s", app.timeout)
	}
}

// ---- ServiceComponent 测试 ----

// TestServiceComponent 测试服务组件
func TestServiceComponent(t *testing.T) {
	comp := NewServiceComponent()
	if comp.Name() != "service" {
		t.Fatalf("Name = %q, want %q", comp.Name(), "service")
	}
	if len(comp.Depends()) != 1 || comp.Depends()[0] != "server" {
		t.Fatalf("Depends = %v, want [server]", comp.Depends())
	}
}

// ---- RegisterAfterStart 测试 ----

// TestRegister_AfterStart 测试启动后注册返回错误
func TestRegister_AfterStart(t *testing.T) {
	c := NewContainer()
	_ = c.Register(&mockComponent{name: "a", provide: 1})
	_ = c.Start(context.Background())

	err := c.Register(&mockComponent{name: "b", provide: 2})
	if err == nil {
		t.Fatal("should not allow register after start")
	}
	_ = c.Stop(context.Background())
}

// ---- StopNotStarted 测试 ----

// TestStop_NotStarted 测试停止未启动的容器不报错
func TestStop_NotStarted(t *testing.T) {
	c := NewContainer()
	err := c.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop on unstarted container should return nil, got %v", err)
	}
}
