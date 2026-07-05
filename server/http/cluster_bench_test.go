package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/middleware/recovery"
)

// server/http 是每个 HTTP 请求的必经路径，本 bench 量化：
//   - clusterInjector：每请求的 cluster + baggage 解析开销（默认装配路径）
//   - ChainHandler：中间件链适配 http.Handler 的开销
//   - FullRequest：完整请求路径（clusterInjector + ChainHandler + 业务 handler）
//
// 默认装配（L1/L2）下 clusterInjector 自动启用；ChainHandler 在用户用 middleware.NewChain
// 显式包装时生效。bench 数据用于防回归与量化默认装配基线。

// benchInterceptor 透传拦截器：仅测量链调度开销，不含业务逻辑
type benchInterceptor struct{}

func (benchInterceptor) Intercept(ctx context.Context, req middleware.Request, h middleware.Handler) (middleware.Response, error) {
	return h(ctx, req)
}
func (benchInterceptor) Name() string { return "bench" }

// BenchmarkClusterInjector_Default 无 cluster header（绝大多数请求，走 default 快速路径）
func BenchmarkClusterInjector_Default(b *testing.B) {
	h := clusterInjector(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

// BenchmarkClusterInjector_WithCluster 带 X-Zeus-Cluster header（灰度流量）
func BenchmarkClusterInjector_WithCluster(b *testing.B) {
	h := clusterInjector(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("X-Zeus-Cluster", "canary")
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

// BenchmarkClusterInjector_WithBaggage cluster + baggage header（cluster + 多 K-V）
func BenchmarkClusterInjector_WithBaggage(b *testing.B) {
	h := clusterInjector(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("X-Zeus-Cluster", "canary")
	req.Header.Set("Baggage", "tenant.id=acme,feature.flag=beta")
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

// BenchmarkChainHandler_EmptyChain 空链（量化 ChainHandler 自身包装开销）
func BenchmarkChainHandler_EmptyChain(b *testing.B) {
	chain := middleware.NewChain()
	h := ChainHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), chain)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

// BenchmarkChainHandler_With3Interceptors 3 层中间件（典型：recovery + tracing + metrics）
func BenchmarkChainHandler_With3Interceptors(b *testing.B) {
	chain := middleware.NewChain(benchInterceptor{}, benchInterceptor{}, benchInterceptor{})
	h := ChainHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), chain)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

// BenchmarkFullRequest 完整请求路径（clusterInjector + ChainHandler[recovery] + 业务 handler）
func BenchmarkFullRequest(b *testing.B) {
	chain := middleware.NewChain(recovery.New())
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := clusterInjector(ChainHandler(inner, chain))
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("X-Zeus-Cluster", "canary")
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}
