package accesslog

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-zeus/zeus/middleware/requestid"
)

func TestHTTPMiddleware_RecordsRequest(t *testing.T) {
	// 用一个 noop log.Writer 避免 stdout 噪音
	// （log 包默认有 fallback，不需要显式注入）

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	})
	h := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Errorf("status = %d, want 201 (should pass through)", rec.Code)
	}
}

func TestHTTPMiddleware_PreservesStatusDefault(t *testing.T) {
	// handler 不显式 WriteHeader，默认应是 200
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	h := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name string
		xff  string
		xri  string
		addr string
		want string
	}{
		{"XFF first wins", "1.1.1.1, 2.2.2.2", "", "", "1.1.1.1"},
		{"XFF single", "9.9.9.9", "", "", "9.9.9.9"},
		{"XRI fallback", "", "8.8.8.8", "", "8.8.8.8"},
		{"RemoteAddr last", "", "", "5.5.5.5:1234", "5.5.5.5:1234"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if c.xri != "" {
				req.Header.Set("X-Real-IP", c.xri)
			}
			req.RemoteAddr = c.addr
			if got := clientIP(req); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestHTTPMiddleware_WorksWithRequestID(t *testing.T) {
	// accesslog 从 ctx 读 request ID，配合 requestid 中间件能取到
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	// 顺序：requestid 外层 → accesslog 内层（accesslog 才能读到 id）
	h := requestid.HTTPMiddleware(HTTPMiddleware(next))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// 验证不 panic + 状态码透传（日志输出由 log 包接管，这里不验证 stdout）
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get(requestid.HeaderRequestID) == "" {
		t.Error("request id header should be set")
	}
}

// TestStatusRecorder_FlushPassthrough 验证 statusRecorder 透传 http.Flusher：
// SSE 等流式场景依赖 w.(http.Flusher) 断言，包装层缺失 Flush 会导致
// 断言失败、流式响应被全量缓冲
func TestStatusRecorder_FlushPassthrough(t *testing.T) {
	// httptest.ResponseRecorder 原生实现 http.Flusher
	inner := httptest.NewRecorder()
	// 经接口变量断言：与真实使用一致（statusRecorder 作为 http.ResponseWriter 传给下游 handler）
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: 200}

	f, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder should implement http.Flusher")
	}

	w.WriteHeader(200)
	w.Write([]byte("event: ping\n\n"))
	f.Flush()

	if inner.Flushed {
		// ResponseRecorder.Flush() 会置 Flushed=true，验证委托真正触达底层
	} else {
		t.Fatal("Flush should delegate to underlying ResponseWriter")
	}
}

// TestStatusRecorder_FlushNoopWithoutFlusher 底层 writer 不实现 Flusher 时
// Flush 应静默 no-op 而非 panic
func TestStatusRecorder_FlushNoopWithoutFlusher(t *testing.T) {
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: nonFlusherWriter{}, status: 200}
	f, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder should implement http.Flusher")
	}
	f.Flush() // 不应 panic
}

// nonFlusherWriter 仅实现 Write/WriteHeader 的最小 ResponseWriter（不实现 Flusher）
type nonFlusherWriter struct{}

func (nonFlusherWriter) Header() http.Header         { return http.Header{} }
func (nonFlusherWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nonFlusherWriter) WriteHeader(int)             {}
