package components

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/middleware/recovery"
	zeushttp "github.com/go-zeus/zeus/server/http"
)

// recordingInterceptor 记录自己被调用的顺序，用于断言多个中间件按拓扑序生效
type recordingInterceptor struct {
	name  string
	calls *[]string
}

func (r *recordingInterceptor) Intercept(ctx context.Context, req middleware.Request, next middleware.Handler) (middleware.Response, error) {
	*r.calls = append(*r.calls, "before:"+r.name)
	resp, err := next(ctx, req)
	*r.calls = append(*r.calls, "after:"+r.name)
	return resp, err
}

func (r *recordingInterceptor) Name() string { return r.name }

// waitPortReady 轮询 TCP 端口直到 server 真正监听（最多 2s），
// 避免 HTTP Get 比 server.Serve 早导致的 connection refused。
func waitPortReady(t *testing.T, port int) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ln, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = ln.Close()
			return
		}
	}
	t.Fatalf("waitPortReady: port %d not listening after 2s", port)
}

// TestGetAllByType_MultipleInstances 验证 GetAllByType 返回所有同接口类型实例（按注册顺序）
// 这是 ServerComponent 自动应用中间件的关键前提
func TestGetAllByType_MultipleInstances(t *testing.T) {
	ctx := newAssemblyContext()
	ctx.set("mw_a", middleware.Interceptor(&recordingInterceptor{name: "a"}))
	ctx.set("mw_b", middleware.Interceptor(&recordingInterceptor{name: "b"}))
	ctx.set("mw_c", middleware.Interceptor(&recordingInterceptor{name: "c"}))
	ctx.set("other", 42) // 干扰项

	got, err := AllByType[middleware.Interceptor](ctx)
	if err != nil {
		t.Fatalf("GetAllByType: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 interceptors, got %d", len(got))
	}
	// 按 set 顺序返回（顺序敏感：ServerComponent 自动应用依赖此约定）
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Name() != want {
			t.Errorf("got[%d].Name() = %q, want %q", i, got[i].Name(), want)
		}
	}
}

// TestGetAllByType_Empty 无注册时返回空切片 + nil
func TestGetAllByType_Empty(t *testing.T) {
	ctx := newAssemblyContext()
	got, err := AllByType[middleware.Interceptor](ctx)
	if err != nil {
		t.Fatalf("GetAllByType: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 interceptors, got %d", len(got))
	}
}

// TestServerComponent_AutoAppliesMiddlewares 验证 MiddlewareComponent 注册的中间件
// 被 ServerComponent.OnStart 自动收集并应用到每个 server（修复 P0 bug）
//
// 顺序说明：Container 拓扑排序按组件名字典序生效（见 resolve.go），
// MiddlewareComponent 的组件名是 "middleware_<interceptor.Name()>"。
// 因此 GetAllByType 返回顺序 = 字典序，对应 Chain 顺序 = 字典序。
// 这里用 mw_a / mw_b 命名让字典序 == 注册顺序，便于阅读。
func TestServerComponent_AutoAppliesMiddlewares(t *testing.T) {
	port := freePort(t)

	var calls []string
	userHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "handler")
		_, _ = w.Write([]byte("ok"))
	})

	srv := zeushttp.NewHTTP(
		zeushttp.Port(port),
		zeushttp.Mux(userHandler),
		zeushttp.WithoutAutoClustering(),
	)

	app := NewApp(
		NewMiddlewareComponent(&recordingInterceptor{name: "mw_a", calls: &calls}),
		NewMiddlewareComponent(&recordingInterceptor{name: "mw_b", calls: &calls}),
		NewServerComponent(srv),
	)

	if err := app.container.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = app.container.Stop(context.Background()) }()
	waitPortReady(t, port)

	resp, err := http.Get("http://127.0.0.1:" + itoa(port) + "/")
	if err != nil {
		t.Fatalf("HTTP Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Chain 顺序 [mw_a, mw_b]，mw_a 在最外层（先进入、最后退出）
	want := []string{
		"before:mw_a",
		"before:mw_b",
		"handler",
		"after:mw_b",
		"after:mw_a",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("calls[%d] = %q, want %q (full: %v)", i, calls[i], w, calls)
			break
		}
	}
}

// TestServerComponent_RecoveryMiddlewareCaught_Panic 验证自动应用的 recovery 中间件
// 能捕获业务 handler 的 panic，避免进程崩溃
func TestServerComponent_RecoveryMiddlewareCaught_Panic(t *testing.T) {
	port := freePort(t)

	userHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	srv := zeushttp.NewHTTP(
		zeushttp.Port(port),
		zeushttp.Mux(userHandler),
		zeushttp.WithoutAutoClustering(),
	)

	app := NewApp(
		NewMiddlewareComponent(recovery.New()),
		NewServerComponent(srv),
	)

	if err := app.container.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = app.container.Stop(context.Background()) }()
	waitPortReady(t, port)

	resp, err := http.Get("http://127.0.0.1:" + itoa(port) + "/")
	if err != nil {
		t.Fatalf("HTTP Get: %v", err)
	}
	defer resp.Body.Close()

	// recovery 中间件把 panic 转成 500，进程不崩
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (recovery should catch panic)", resp.StatusCode)
	}
}

// TestServerComponent_NoMiddleware_NoOp 验证无 MiddlewareComponent 时
// ServerComponent.OnStart 跳过中间件注入，server 正常运行
func TestServerComponent_NoMiddleware_NoOp(t *testing.T) {
	port := freePort(t)

	srv := zeushttp.NewHTTP(
		zeushttp.Port(port),
		zeushttp.Mux(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(204)
		})),
		zeushttp.WithoutAutoClustering(),
	)

	app := NewApp(NewServerComponent(srv))
	if err := app.container.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = app.container.Stop(context.Background()) }()
	waitPortReady(t, port)

	resp, err := http.Get("http://127.0.0.1:" + itoa(port) + "/")
	if err != nil {
		t.Fatalf("HTTP Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

// itoa 简化 strconv.Itoa 调用，避免在测试文件里多一个 import
func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = digits[n%10]
		n /= 10
	}
	return string(buf[pos:])
}
