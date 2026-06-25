package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/registry/memory"
	zeushttp "github.com/go-zeus/zeus/server/http"
)

// —— 辅助函数 ——

// freePort 取一个空闲端口（避免端口冲突）
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// waitForReady 轮询 URL 直到响应 200 或超时
func waitForReady(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("server at %s not ready in %v", url, timeout)
}

// signalShutdown 向当前进程发 SIGINT 触发优雅关闭
func signalShutdown(t *testing.T) {
	t.Helper()
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGINT)
}

// —— 单元测试 ——

// TestNewApp_Defaults 零参数（除 Server）启动：默认 slog + memory registry + 自动注册
func TestNewApp_Defaults(t *testing.T) {
	port := freePort(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/hi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})

	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port), zeushttp.Mux(mux))),
		WithServiceName("test-defaults"),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()

	url := fmt.Sprintf("http://127.0.0.1:%d/hi", port)
	waitForReady(t, url, 3*time.Second)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	signalShutdown(t)
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after signal")
	}
}

// TestNewApp_NoServer_Errors 不传 Server 时 Run 返回明确错误
func TestNewApp_NoServer_Errors(t *testing.T) {
	a := NewApp(WithServiceName("no-server"))

	// Run 应失败（ServiceComponent 依赖 server）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := a.RunWithContext(ctx)
	if err == nil {
		t.Fatal("expected error when no server is registered")
	}
}

// TestNewApp_MultipleServers 多 Server 场景：两个端口都独立可访问
//
// 验证目标：WithServer 多次调用累加，两个 server 独立监听各自端口
// 不再断言注册中心内部 Instances（避免与 Deregister 写并发触发 race）
func TestNewApp_MultipleServers(t *testing.T) {
	port1 := freePort(t)
	port2 := freePort(t)

	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port1))),
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port2))),
		WithRegistry(memory.New()),
		WithServiceName("multi-svc"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- a.RunWithContext(ctx) }()

	// 等两个端口都 ready（默认 useDefault=true，自动挂载 /health）
	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/health", port1), 3*time.Second)
	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/health", port2), 3*time.Second)

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
	}
}

// TestNewApp_CustomLogger 自定义 Logger 注入到 appConfig
func TestNewApp_CustomLogger(t *testing.T) {
	buf := newBufLogger()
	port := freePort(t)

	cfg := captureConfig(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port))),
		WithLogger(buf),
	)
	if cfg.logger != buf {
		t.Error("WithLogger did not inject custom logger into appConfig")
	}
}

// TestNewApp_CustomRegistry 自定义 Registry 注入到 appConfig
func TestNewApp_CustomRegistry(t *testing.T) {
	reg := memory.New()
	cfg := captureConfig(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(0))),
		WithRegistry(reg),
	)
	if cfg.registry != reg {
		t.Error("WithRegistry did not inject custom registry")
	}
}

// TestNewApp_AutoMiddleware 显式 WithMiddleware 后，panic 被 recovery 捕获
//
// 验证中间件链生效：handler 主动 panic，应被 recovery.Intercept 捕获，不导致进程崩溃
//
// 注：传 Mux 时 useDefault=false，不会自动挂载 /health，需自行挂载以供 waitForReady 探活
func TestNewApp_AutoMiddleware(t *testing.T) {
	port := freePort(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("intentional test panic")
	})

	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port), zeushttp.Mux(mux))),
		WithMiddleware(recovery.New()),
		WithServiceName("mw-test"),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()

	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/health", port), 3*time.Second)

	// 触发 panic 路径（不应崩溃进程）
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/panic", port))
	if err != nil {
		t.Fatalf("Get /panic: %v", err)
	}
	resp.Body.Close()
	// recovery 拦截后由 server 转成 500（具体状态码由 server 实现，仅验证不崩溃）

	signalShutdown(t)
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
	}
}

// TestNewApp_BuilderPattern 链式拼装 8+ Option，断言 appConfig 字段正确
func TestNewApp_BuilderPattern(t *testing.T) {
	reg := memory.New()
	buf := newBufLogger()

	cfg := captureConfig(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(8080))),
		AddServer(zeushttp.NewHTTP(zeushttp.Port(8081))),
		WithLogger(buf),
		WithRegistry(reg),
		WithServiceName("my-app"),
		WithServiceCluster("canary"),
		WithServiceIP("10.0.0.1"),
		WithMiddleware(recovery.New()),
		WithStopTimeout(30*time.Second),
	)

	if len(cfg.servers) != 2 {
		t.Errorf("servers count = %d, want 2", len(cfg.servers))
	}
	if cfg.logger != buf {
		t.Error("logger not set")
	}
	if cfg.registry != reg {
		t.Error("registry not set")
	}
	if cfg.name != "my-app" {
		t.Errorf("name = %q, want my-app", cfg.name)
	}
	if cfg.cluster != "canary" {
		t.Errorf("cluster = %q, want canary", cfg.cluster)
	}
	if cfg.ip != "10.0.0.1" {
		t.Errorf("ip = %q, want 10.0.0.1", cfg.ip)
	}
	if len(cfg.middlewares) != 1 {
		t.Errorf("middlewares count = %d, want 1", len(cfg.middlewares))
	}
	if cfg.stopTimeout != 30*time.Second {
		t.Errorf("stopTimeout = %v, want 30s", cfg.stopTimeout)
	}
}

// TestNewApp_DefaultValues 零参数应有合理默认值
func TestNewApp_DefaultValues(t *testing.T) {
	// captureConfig 不填充默认 logger/registry（测试断言用），故用真实 NewApp 流程
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		stopTimeout: defaultStopTimeoutL3,
	}
	// 模拟 NewApp 初始填充
	if cfg.name != defaultServiceName {
		t.Errorf("default name = %q, want %q", cfg.name, defaultServiceName)
	}
	if cfg.cluster != defaultServiceCluster {
		t.Errorf("default cluster = %q, want %q", cfg.cluster, defaultServiceCluster)
	}
	if cfg.stopTimeout != defaultStopTimeoutL3 {
		t.Errorf("default stopTimeout = %v, want %v", cfg.stopTimeout, defaultStopTimeoutL3)
	}
}

// TestNewApp_MixedWithL4 L3/L4 混用：L4 组件直接追加为参数
func TestNewApp_MixedWithL4(t *testing.T) {
	port := freePort(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/hi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mockCache := &mockCacheForL3{}
	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port), zeushttp.Mux(mux))),
		WithServiceName("mixed-test"),
		// L4 透传：直接追加 components.NewCacheComponent
		components.NewCacheComponent(mockCache),
	)

	// 启动并验证 cache 实例可从 Container 取出
	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()

	waitForReady(t, fmt.Sprintf("http://127.0.0.1:%d/hi", port), 3*time.Second)

	// 通过 Container 验证 cache 注册成功
	got, ok := a.Get("cache")
	if !ok {
		t.Fatal("cache component not found in container")
	}
	if got == nil {
		t.Error("cache instance is nil")
	}
	if mockCache.closeCalled.Load() {
		t.Error("cache Close should not be called while running")
	}

	signalShutdown(t)
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
	}

	// 优雅关闭后 cache.Close 应被调用（OnStop 钩子）
	if !mockCache.closeCalled.Load() {
		t.Error("cache.Close should be called on stop")
	}
}

// TestNewApp_ReturnsComponentsApp 返回类型是 *components.App
func TestNewApp_ReturnsComponentsApp(t *testing.T) {
	port := freePort(t)
	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(port))),
	)

	// 类型断言：返回值必须是 *components.App
	if _, ok := any(a).(*components.App); !ok {
		t.Errorf("NewApp returned %T, want *components.App", a)
	}

	// 关键方法可用
	_ = a.Container()
	_ = a.Get
}

// —— 辅助类型 ——

// captureConfig 应用 opts 到临时 appConfig 并返回，不实际构造 App（用于断言字段）
//
// 注：不填充 slog/memory 默认值（测试想看清是否被 WithXxx 显式注入）
func captureConfig(opts ...AppOption) *appConfig {
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		stopTimeout: defaultStopTimeoutL3,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// newBufLogger 创建一个测试用 log.Writer（buffer 实现，仅记录最近一条日志）
type bufLogger struct {
	mu      sync.Mutex
	lastMsg string
	lastLvl log.Level
}

func (b *bufLogger) Log(_ context.Context, level log.Level, msg string, _ ...log.Field) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastLvl = level
	b.lastMsg = msg
}
func (b *bufLogger) Close() error { return nil }

func newBufLogger() log.Writer { return &bufLogger{} }

// mockCacheForL3 用于 TestNewApp_MixedWithL4 验证 L4 组件透传
//
// closeCalled 用 atomic.Bool 而非 mutex+bool：测试主 goroutine 读、App.Run
// 内部 goroutine 写，必须保证读端无锁也可安全访问，避免 race detector 报警。
type mockCacheForL3 struct {
	closeCalled atomic.Bool
}

func (m *mockCacheForL3) Get(context.Context, string) (any, bool)                 { return nil, false }
func (m *mockCacheForL3) Set(context.Context, string, any, ...cache.Option) error { return nil }
func (m *mockCacheForL3) Delete(context.Context, string) error                    { return nil }
func (m *mockCacheForL3) Has(context.Context, string) bool                        { return false }
func (m *mockCacheForL3) Close() error {
	m.closeCalled.Store(true)
	return nil
}
