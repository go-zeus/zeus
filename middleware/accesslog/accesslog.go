// Package accesslog 记录 HTTP 请求日志（method/path/status/duration/requestID）。
//
// 行为：
//   - 包装 http.Handler，每次请求结束打印一行
//   - 自动从 context 读取 request ID（需配合 requestid 中间件）
//   - 用 zeus log.Writer，跟应用日志统一格式
package accesslog

import (
	"net/http"
	"time"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/middleware/requestid"
)

// statusRecorder 包装 ResponseWriter 捕获状态码
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware HTTP 风格中间件，记录每个请求
//
// 用法：
//
//	handler := accesslog.HTTPMiddleware(myHandler)
//	// 或跟 requestid 组合
//	handler := requestid.HTTPMiddleware(accesslog.HTTPMiddleware(myHandler))
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		reqID := requestid.FromContext(r.Context())

		log.Info("req %s %s status=%d duration=%s ip=%s request_id=%s",
			r.Method,
			r.URL.Path,
			rec.status,
			duration,
			clientIP(r),
			reqID,
		)
	})
}

// clientIP 提取真实客户端 IP（X-Forwarded-For 优先）
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// 取第一个 IP（链路最左是原始 client）
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
