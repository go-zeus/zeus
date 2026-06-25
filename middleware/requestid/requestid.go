// Package requestid 为每个请求注入唯一 ID（X-Request-ID Header）。
//
// 行为：
//   - 请求带 X-Request-ID → 沿用
//   - 请求未带 → 生成 uuid
//   - 通过 context 传递（routing.WithCluster 模式），让 log/trace 自动带上
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-zeus/zeus/middleware"
)

// HeaderRequestID HTTP Header 名
const HeaderRequestID = "X-Request-ID"

// ctxKey context key 类型（避免冲突）
type ctxKey struct{}

// 编译期检查 requestIDInterceptor 实现 middleware.Interceptor
var _ middleware.Interceptor = (*requestIDInterceptor)(nil)

type requestIDInterceptor struct{}

// New 创建 requestid 中间件
func New() middleware.Interceptor {
	return &requestIDInterceptor{}
}

func (r *requestIDInterceptor) Intercept(ctx context.Context, req middleware.Request, handler middleware.Handler) (middleware.Response, error) {
	id := req.Header(HeaderRequestID)
	if id == "" {
		id = generateID()
	}
	ctx = WithID(ctx, id)
	return handler(ctx, req)
}

func (r *requestIDInterceptor) Name() string { return "requestid" }

// WithID 把 request ID 注入 context
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext 从 context 读取 request ID（不存在返回空字符串）
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// HTTPMiddleware 提供 http.Handler 风格的中间件（不依赖 middleware.Interceptor 的场景用）
//
// 用法：handler := requestid.HTTPMiddleware(myHandler)
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = generateID()
			// 注入回 Header，让下游能看到
			r.Header.Set(HeaderRequestID, id)
		}
		// 总是回写到 response，让客户端能拿到 request id
		w.Header().Set(HeaderRequestID, id)
		ctx := WithID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateID 生成 16 字节 hex 编码的随机 ID（32 字符）
// 用 crypto/rand 避免 uuid 包依赖
func generateID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
