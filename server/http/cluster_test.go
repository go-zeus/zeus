package http

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
)

// freePort 获取系统当前空闲端口（测试基础设施，避免硬编码端口）
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestClusterInjector_DefaultBehavior 验证默认行为：X-Zeus-Cluster Header 自动注入 context
func TestClusterInjector_DefaultBehavior(t *testing.T) {
	var gotCluster string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCluster = routing.FromContext(r.Context())
		w.WriteHeader(200)
	})

	wrapped := clusterInjector(h)
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(routing.HeaderCluster, "canary")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	if gotCluster != "canary" {
		t.Fatalf("got cluster %q, want canary", gotCluster)
	}
}

// TestClusterInjector_MissingHeader_FallsBackToDefault 验证缺失 Header 时回退到 Default
func TestClusterInjector_MissingHeader_FallsBackToDefault(t *testing.T) {
	var gotCluster string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCluster = routing.FromContext(r.Context())
	})
	wrapped := clusterInjector(h)

	r := httptest.NewRequest("GET", "/x", nil)
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	if gotCluster != routing.Default {
		t.Fatalf("got %q, want %q", gotCluster, routing.Default)
	}
}

// TestClusterInjector_DoesNotMutateOriginalRequest 验证不修改原请求
func TestClusterInjector_DoesNotMutateOriginalRequest(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟下游处理
	})
	wrapped := clusterInjector(h)

	r := httptest.NewRequest("GET", "/x", nil)
	originalCtx := r.Context()
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	// 原请求 context 不变（注入只发生在 wrapper 内的 WithContext 派生 ctx）
	if r.Context() != originalCtx {
		t.Fatal("clusterInjector should not mutate original request context")
	}
}

// TestClusterInjector_ExtractsBaggage clusterInjector 同时解析 W3C Baggage Header
// 用户自定义 K-V 自动注入到 ctx，业务 handler 通过 propagation.Get 读取
func TestClusterInjector_ExtractsBaggage(t *testing.T) {
	var gotTenant, gotFeature string
	var hasTenant, hasFeature bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant, hasTenant = propagation.Get(r.Context(), "tenant.id")
		gotFeature, hasFeature = propagation.Get(r.Context(), "feature.flag")
		w.WriteHeader(200)
	})

	wrapped := clusterInjector(h)
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(propagation.HeaderBaggage, "tenant.id=acme-corp,feature.flag=beta")
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	if !hasTenant || gotTenant != "acme-corp" {
		t.Errorf("tenant.id = (%q,%v), want (acme-corp,true)", gotTenant, hasTenant)
	}
	if !hasFeature || gotFeature != "beta" {
		t.Errorf("feature.flag = (%q,%v), want (beta,true)", gotFeature, hasFeature)
	}
}

// TestClusterInjector_BaggageAndClusterCoexist baggage 和 X-Zeus-Cluster 同时存在，互不干扰
func TestClusterInjector_BaggageAndClusterCoexist(t *testing.T) {
	var (
		gotCluster      string
		gotFromBaggage  string
		bagClusterHasIt bool
	)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCluster = routing.FromContext(r.Context())
		gotFromBaggage, bagClusterHasIt = propagation.Get(r.Context(), routing.BagKey)
		w.WriteHeader(200)
	})

	wrapped := clusterInjector(h)
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(routing.HeaderCluster, "canary")
	r.Header.Set(propagation.HeaderBaggage, "tenant.id=acme")
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	if gotCluster != "canary" {
		t.Errorf("routing cluster = %q, want canary", gotCluster)
	}
	// routing.WithCluster 会同步写入 propagation Bag，所以从 Bag 也能读到 cluster
	if !bagClusterHasIt || gotFromBaggage != "canary" {
		t.Errorf("propagation[zeus.cluster] = (%q,%v), want (canary,true)", gotFromBaggage, bagClusterHasIt)
	}
}

// TestHTTPServer_AutoClusteringDefault 验证 NewHTTP 默认启用 autoClustering
func TestHTTPServer_AutoClusteringDefault(t *testing.T) {
	s := NewHTTP().(*httpServer)
	if !s.autoClustering {
		t.Error("autoClustering should default to true")
	}
}

// TestHTTPServer_WithoutAutoClustering 验证 WithoutAutoClustering 关闭默认行为
func TestHTTPServer_WithoutAutoClustering(t *testing.T) {
	s := NewHTTP(WithoutAutoClustering()).(*httpServer)
	if s.autoClustering {
		t.Error("autoClustering should be false after WithoutAutoClustering")
	}
}

// TestChainHandler 验证 ChainHandler 把中间件链接入 http.Handler
func TestChainHandler(t *testing.T) {
	var ordered []string

	// 构造测试中间件：观察顺序
	called := &ordered
	traceMW := &recordingInterceptor{name: "trace", called: called}
	metricsMW := &recordingInterceptor{name: "metrics", called: called}

	chain := middleware.NewChain(traceMW, metricsMW)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = append(*called, "handler")
		w.WriteHeader(200)
	})

	h := ChainHandler(next, chain)
	r := httptest.NewRequest("GET", "/x", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)

	// 期望顺序：trace 进入 → metrics 进入 → handler → metrics 退出 → trace 退出
	want := []string{"trace-in", "metrics-in", "handler", "metrics-out", "trace-out"}
	if len(*called) != len(want) {
		t.Fatalf("got %v, want %v", *called, want)
	}
	for i, w := range want {
		if (*called)[i] != w {
			t.Errorf("order[%d] = %q, want %q; full=%v", i, (*called)[i], w, *called)
		}
	}
}

// TestHTTPServer_Start_InjectsCluster 端到端验证：启动 server 后请求带 Header，handler 内 ctx 已含 cluster
func TestHTTPServer_Start_InjectsCluster(t *testing.T) {
	clusterCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		clusterCh <- routing.FromContext(r.Context())
		w.WriteHeader(200)
	})

	// 动态端口避免与开发环境常用端口冲突
	srv := NewHTTP(Mux(mux), Port(freePort(t))).(*httpServer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = srv.Start(ctx) }()
	defer func() {
		shutCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = srv.Stop(shutCtx)
	}()

	// 等待 server 就绪
	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+srv.Endpoint()+"/ping", nil)
	req.Header.Set(routing.HeaderCluster, "canary")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	select {
	case c := <-clusterCh:
		if c != "canary" {
			t.Fatalf("got cluster %q, want canary", c)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for handler invocation")
	}
}

// recordingInterceptor 用于 ChainHandler 测试，记录进入/退出顺序
type recordingInterceptor struct {
	name   string
	called *[]string
}

func (r *recordingInterceptor) Intercept(ctx context.Context, _ middleware.Request, h middleware.Handler) (middleware.Response, error) {
	*r.called = append(*r.called, r.name+"-in")
	resp, err := h(ctx, nil)
	*r.called = append(*r.called, r.name+"-out")
	return resp, err
}

func (r *recordingInterceptor) Name() string { return r.name }

// 编译期检查 recordingInterceptor 实现 middleware.Interceptor
var _ middleware.Interceptor = (*recordingInterceptor)(nil)
