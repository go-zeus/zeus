package propagation_test

// e2e_test.go 端到端集成测试：模拟 HTTP 上游 → zeus server → zeus client → 下游 server
// 全链路 baggage 自动透传。
//
// 使用 external test package (propagation_test) 而非 internal (propagation)，
// 因为 client 包导入了 propagation，若用 internal package 会形成循环依赖。
//
// 测试架构：
//
//	[callee2 server]  ← 真实 HTTP server，clusterInjector + 业务 handler
//	       ▲
//	       │ client.Do（zeus client，自动 InjectHTTP）
//	       │
//	[callee1 server]  ← 真实 HTTP server，clusterInjector + 业务 handler
//	       ▲
//	       │ httptest 调用方（模拟上游，携带 Baggage header）
//	       │
//	[test caller]
//
// 验证契约：
//   1. 上游发送 Baggage header → callee1 自动 extract 到 ctx
//   2. callee1 handler 中调 zeus client.Do(ctx, req) → 自动 InjectHTTP 透传
//   3. callee2 收到 Baggage header（应包含上游的所有 K-V + callee1 注入的新 K-V）

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	zclient "github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/routing"
	zeushttp "github.com/go-zeus/zeus/server/http"
	"github.com/go-zeus/zeus/types"
)

// freePort 获取空闲端口
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startServer 启动一个带 clusterInjector 的真实 zeus HTTP server。
//
// 返回 server URL；测试结束自动关闭。
func startServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	port := freePort(t)

	chain := middleware.NewChain(recovery.New())
	srv := zeushttp.NewHTTP(
		zeushttp.Mux(zeushttp.ChainHandler(handler, chain)),
		zeushttp.Port(port),
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = srv.Stop(context.Background())
	})

	go func() {
		_ = srv.Start(ctx)
	}()

	// 等待端口就绪
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 60; i++ {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return "http://" + addr
		}
	}
	t.Fatalf("server at %s did not become ready", addr)
	return ""
}

// TestE2E_BaggageEndToEnd 全链路 baggage 自动透传
//
// 场景：
//  1. caller 发请求带 Baggage: tenant.id=acme,feature.flag=beta
//  2. callee1 入口自动 extract，业务读到 tenant=acme / feature=beta
//  3. callee1 业务注入新 K-V: user.tier=premium
//  4. callee1 调 zeus client.Do(ctx, req) 访问 callee2
//  5. callee2 入口自动 extract，应同时看到 tenant.id / feature.flag / user.tier
func TestE2E_BaggageEndToEnd(t *testing.T) {
	// === callee2：被依赖的下游服务 ===
	var callee2Received map[string]string
	callee2Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		callee2Received = map[string]string{
			"tenant.id":    getOrDefault(ctx, "tenant.id"),
			"feature.flag": getOrDefault(ctx, "feature.flag"),
			"user.tier":    getOrDefault(ctx, "user.tier"),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("callee2 ok"))
	})
	callee2URL := startServer(t, callee2Handler)
	callee2Host, callee2Port := parseHostPort2(t, callee2URL)

	// === callee1：中间服务（接收 caller 请求，再调 callee2） ===
	callee1Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1) 验证上游 caller 的 baggage 已自动 extract
		tenant, _ := propagation.Get(ctx, "tenant.id")
		feature, _ := propagation.Get(ctx, "feature.flag")
		if tenant != "acme" {
			t.Errorf("callee1: tenant.id = %q, want acme", tenant)
		}
		if feature != "beta" {
			t.Errorf("callee1: feature.flag = %q, want beta", feature)
		}

		// 2) 业务注入新 K-V（典型场景：根据 user token 决定 user.tier）
		ctx = propagation.With(ctx, "user.tier", "premium")

		// 3) 调下游 callee2：构造 req，从 ctx 注入 baggage（zeus client 自动 InjectHTTP）
		downstreamReq, _ := http.NewRequest("GET", callee2URL+"/downstream", nil)
		downstreamReq = downstreamReq.WithContext(ctx)
		zeusClient := newZeusClient(t, "callee2-service", callee2Host, callee2Port)
		resp, err := zeusClient.Do(downstreamReq)
		if err != nil {
			t.Errorf("callee1 → callee2 call failed: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	callee1URL := startServer(t, callee1Handler)

	// === caller：模拟上游，发请求带 Baggage header ===
	req, _ := http.NewRequest("GET", callee1URL+"/api", nil)
	req.Header.Set(propagation.HeaderBaggage, "tenant.id=acme,feature.flag=beta")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("caller request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// 验证 callee2 收到全部 3 个 K-V（caller 2 个 + callee1 注入 1 个）
	if callee2Received == nil {
		t.Fatal("callee2 did not receive request")
	}
	if callee2Received["tenant.id"] != "acme" {
		t.Errorf("callee2 tenant.id = %q, want acme", callee2Received["tenant.id"])
	}
	if callee2Received["feature.flag"] != "beta" {
		t.Errorf("callee2 feature.flag = %q, want beta", callee2Received["feature.flag"])
	}
	if callee2Received["user.tier"] != "premium" {
		t.Errorf("callee2 user.tier = %q, want premium (injected by callee1)",
			callee2Received["user.tier"])
	}
}

// TestE2E_ClusterAndBaggagePropagatedTogether cluster + 自定义 baggage 同时透传
//
// 验证 routing.WithCluster 与 baggage 在跨进程链路中协同工作。
func TestE2E_ClusterAndBaggagePropagatedTogether(t *testing.T) {
	var calleeReceived map[string]string
	var calleeCluster string

	calleeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		calleeCluster = routing.FromContext(ctx)
		calleeReceived = map[string]string{
			"tenant.id": getOrDefault(ctx, "tenant.id"),
		}
		w.WriteHeader(http.StatusOK)
	})
	calleeURL := startServer(t, calleeHandler)
	host, port := parseHostPort2(t, calleeURL)

	callerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// 显式设置 cluster（同时写入 ctx 和 baggage）
		ctx = routing.WithCluster(ctx, "canary")
		// 注入用户 K-V
		ctx = propagation.With(ctx, "tenant.id", "globex")

		req, _ := http.NewRequest("GET", calleeURL+"/x", nil)
		req = req.WithContext(ctx)
		zeusClient := newZeusClient(t, "callee-svc", host, port)
		_, _ = zeusClient.Do(req)
		w.WriteHeader(http.StatusOK)
	})
	callerURL := startServer(t, callerHandler)

	// caller 不传 baggage，让 cluster 注入触发
	req, _ := http.NewRequest("GET", callerURL+"/start", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("caller request failed: %v", err)
	}
	defer resp.Body.Close()

	if calleeCluster != "canary" {
		t.Errorf("callee cluster = %q, want canary", calleeCluster)
	}
	if calleeReceived["tenant.id"] != "globex" {
		t.Errorf("callee tenant.id = %q, want globex", calleeReceived["tenant.id"])
	}
}

// TestE2E_NoBaggageHeaderSafe 上游不带 baggage header，下游 ctx 仍能正常工作
//
// 验证：缺失 Baggage header 时不应 panic，业务读不到 K-V 是 (false)。
func TestE2E_NoBaggageHeaderSafe(t *testing.T) {
	calleeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if _, ok := propagation.Get(ctx, "tenant.id"); ok {
			t.Error("should not have tenant.id when no Baggage header sent")
		}
		w.WriteHeader(http.StatusOK)
	})
	calleeURL := startServer(t, calleeHandler)

	req, _ := http.NewRequest("GET", calleeURL+"/x", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// getOrDefault 从 ctx 读取 baggage 值，缺失返回空串
func getOrDefault(ctx context.Context, key string) string {
	v, _ := propagation.Get(ctx, key)
	return v
}

// parseHostPort2 从 URL 解析出 host 和 port
func parseHostPort2(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host := u.Hostname()
	port := 80
	fmt.Sscanf(u.Port(), "%d", &port)
	return host, port
}

// newZeusClient 创建一个 zeus client，注册到内存注册中心
func newZeusClient(t *testing.T, name, ip string, port int) zclient.HTTPClient {
	t.Helper()
	reg := memory.New()
	// memory 同时实现 Registrar 和 Discovery，但 New() 返回类型声明为 Registrar
	// 需要类型断言拿 Discovery 接口
	dis, ok := reg.(registry.Discovery)
	if !ok {
		t.Fatalf("memory should implement Discovery")
	}
	ctx := context.Background()
	// 注册 default 和 canary 两个 cluster，让 client.Do fallback 能成功
	for _, c := range []string{"default", "canary"} {
		_ = reg.Register(ctx, &types.Instance{
			ID:      fmt.Sprintf("%s-%s-%d", name, c, port),
			Name:    name,
			Cluster: c,
			IP:      ip,
			Port:    port,
		})
	}
	return zclient.NewClient(name,
		zclient.Discovery(dis),
		zclient.LoadBalance(roundrobin.New()),
	)
}
