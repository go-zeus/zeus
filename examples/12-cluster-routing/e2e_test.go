package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/proxy"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

// TestE2E_HTTP_ClusterRouting 端到端测试：gateway → srv1 → srv2 完整集群路由
//
// 验证场景：
//   - X-Zeus-Cluster: canary → srv1[canary] → srv2[canary]
//   - 无 header    → srv1[default] → srv2[default]
func TestE2E_HTTP_ClusterRouting(t *testing.T) {
	ctx := context.Background()
	reg := memory.New()
	dis := reg.(registry.Discovery)

	// === 启动 srv2 的 default 和 canary 后端（用 httptest） ===
	srv2Default := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 srv2 收到的 ctx 已含 cluster
		c := routing.FromContext(r.Context())
		// httptest 的 server 不会自动注入 ctx，需通过中间件
		// 这里只是简单返回标识
		_, _ = io.WriteString(w, "srv2[default,cluster="+c+"]")
	}))
	defer srv2Default.Close()

	srv2Canary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "srv2[canary,cluster="+r.Header.Get(routing.HeaderCluster)+"]")
	}))
	defer srv2Canary.Close()

	// 注册 srv2 实例（default 和 canary 各一）
	reg.Register(ctx, makeInstance("srv2-default", "srv2", routing.Default, srv2Default.Listener.Addr().String()))
	reg.Register(ctx, makeInstance("srv2-canary", "srv2", "canary", srv2Canary.Listener.Addr().String()))

	// === 启动 srv1：用 client 调 srv2 ===
	srv2Client := client.NewClient("srv2",
		client.Discovery(dis),
		client.LoadBalance(roundrobin.New()),
	)
	// Close 方法在底层 *client struct 上，通过类型断言调用
	if closer, ok := srv2Client.(interface{ Close() }); ok {
		defer closer.Close()
	}

	// srv1 的 handler：从 ctx 读取 cluster（被 server/http 注入），调下游 srv2
	srv1Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// clusterInjector 中间件已注入 ctx
		c := routing.FromContext(r.Context())

		// 用 client 调 srv2（client.Do 从 ctx 读 cluster 路由）
		req, _ := http.NewRequestWithContext(r.Context(), "GET", "http://srv2/who", nil)
		// 手动透传 X-Zeus-Cluster Header（v1 client 走 HTTP Header 模式）
		if !routing.IsDefault(c) {
			req.Header.Set(routing.HeaderCluster, c)
		}
		resp, err := srv2Client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		_, _ = io.WriteString(w, "srv1["+c+"] -> "+string(body))
	})

	// srv1 用 server/http 风格：套上 clusterInjector
	srv1Srv := httptest.NewServer(clusterInjectorHandler(srv1Handler))
	defer srv1Srv.Close()

	reg.Register(ctx, makeInstance("srv1-default", "srv1", routing.Default, srv1Srv.Listener.Addr().String()))

	// === gateway：proxy 反向代理 + 集群选择器 ===
	gw := proxy.New(proxy.WithSelector(proxy.NewDiscoverySelector("srv1", dis, roundrobin.New())))
	gatewaySrv := httptest.NewServer(gw)
	defer gatewaySrv.Close()

	time.Sleep(200 * time.Millisecond) // 等待 client watcher 拉取实例

	// === 测试 1：canary cluster 路径 ===
	req1, _ := http.NewRequest("GET", gatewaySrv.URL+"/who", nil)
	req1.Header.Set(routing.HeaderCluster, "canary")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("canary request error: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	got1 := string(body1)

	// 期望：srv1[canary] -> srv2[canary,cluster=canary]
	if !strings.Contains(got1, "srv1[canary]") {
		t.Errorf("canary req: missing srv1[canary] in %q", got1)
	}
	if !strings.Contains(got1, "srv2[canary") {
		t.Errorf("canary req: missing srv2[canary] in %q", got1)
	}

	// === 测试 2：default 路径（无 header） ===
	req2, _ := http.NewRequest("GET", gatewaySrv.URL+"/who", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("default request error: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	got2 := string(body2)

	// 期望：srv1[default] -> srv2[default]
	if !strings.Contains(got2, "srv1[default]") {
		t.Errorf("default req: missing srv1[default] in %q", got2)
	}
	if !strings.Contains(got2, "srv2[default") {
		t.Errorf("default req: missing srv2[default] in %q", got2)
	}
}

// makeInstance 从 addr 解析 ip:port 创建 Instance
func makeInstance(id, name, cluster, addr string) *types.Instance {
	host, port := parseAddr(addr)
	return &types.Instance{
		ID:      id,
		Name:    name,
		Cluster: cluster,
		IP:      host,
		Port:    port,
	}
}

// parseAddr 简化的 host:port 解析（避免引入 net.SplitHostPort 复杂度）
func parseAddr(addr string) (string, int) {
	// addr 形如 "127.0.0.1:12345"
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, 0
	}
	host := addr[:idx]
	port := 0
	for i := idx + 1; i < len(addr); i++ {
		port = port*10 + int(addr[i]-'0')
	}
	return host, port
}

// clusterInjectorHandler 复制 server/http 的 clusterInjector（避免循环依赖）
func clusterInjectorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := routing.ClusterFromHTTPHeader(r.Header)
		ctx := routing.WithCluster(r.Context(), c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
