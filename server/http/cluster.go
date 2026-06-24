package http

import (
	"context"
	"net/http"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
)

// clusterInjector 在 HTTP handler 入口做两件事：
//
//  1. 解析 X-Zeus-Cluster Header（显式协议，易调试）写入 ctx
//  2. 解析 W3C Baggage Header（用户自定义 K-V 全链路传播）写入 ctx
//
// 行为：
//   - cluster：从 r.Header 读取，缺失则使用 routing.Default
//   - baggage：调 propagation.ExtractHTTP 解析所有用户自定义 K-V
//   - 调用 next 时 ctx 同时包含 cluster 和 baggage entries
//
// 用途：让业务 handler / client / log / trace 等下游组件自动获得 cluster 标记
// 及用户自定义 K-V（如 tenant.id / feature.flag）。
func clusterInjector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := routing.ClusterFromHTTPHeader(r.Header)
		ctx := routing.WithCluster(r.Context(), c)
		// 追加 extract baggage 中的用户自定义 K-V（cluster 已写入 Bag，重复 Key 会覆盖为同值）
		ctx = propagation.ExtractHTTP(ctx, r.Header)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ChainHandler 把 middleware.Chain 适配为 http.Handler。
//
// 用法：
//
//	chain := middleware.NewChain(clustering.New(), tracing.New(), metrics.New())
//	s := http.NewHTTP(http.Mux(http.ChainHandler(yourMux, chain)))
//
// 说明：
//   - 每次请求构造一个 httpRequest 包装器适配 middleware.Request 接口
//   - 链处理结束后，把响应写回 http.ResponseWriter
//   - 默认响应是 200 + 空 body（业务侧通常通过 next handler 直接写 ResponseWriter）
//   - 如果链返回 error（典型：recovery 中间件捕获到 panic 且 handler 还没写过响应），
//     ChainHandler 兜底写 500，避免客户端收到空 200
func ChainHandler(next http.Handler, chain middleware.Chain) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 用 trackingWriter 跟踪响应是否已写过 header，避免 panic 后重复 WriteHeader
		tw := &trackingWriter{ResponseWriter: w}
		// 业务 handler 仍然走原始 next（直接写 w），中间件只能观察/修改 ctx 与 req
		// 因此这里把 chain 用于"前置处理"，最终调用 next.ServeHTTP
		req := &httpRequest{r: r}
		_, err := chain.Handle(r.Context(), req, func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			next.ServeHTTP(tw, r.WithContext(ctx))
			return nil, nil
		})
		if err != nil && !tw.wroteHeader {
			// handler 在写响应之前 panic / 返回 error：兜底 500
			http.Error(tw, "internal server error", http.StatusInternalServerError)
		}
	})
}

// trackingWriter 包装 http.ResponseWriter，跟踪是否已经写过响应状态。
//
// 典型场景：ChainHandler 借此检测 panic 发生在 handler 写响应之前还是之后，
// 决定日志该打印 status=500 还是 status=200。
type trackingWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *trackingWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *trackingWriter) Write(b []byte) (int, error) {
	// net/http 的 Write 在未显式 WriteHeader 时会隐式调用 WriteHeader(200)
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// httpRequest 适配 middleware.Request 接口
type httpRequest struct {
	r *http.Request
}

func (h *httpRequest) Method() string         { return h.r.Method }
func (h *httpRequest) Path() string           { return h.r.URL.Path }
func (h *httpRequest) Header(k string) string { return h.r.Header.Get(k) }
func (h *httpRequest) Body() any              { return h.r.Body }

// 编译期检查 httpRequest 实现 middleware.Request
var _ middleware.Request = (*httpRequest)(nil)
