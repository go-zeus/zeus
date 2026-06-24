package middleware

import "context"

// Request 中间件请求抽象
type Request interface {
	Method() string
	Path() string
	Header(key string) string
	Body() any
}

// Response 中间件响应抽象
type Response interface {
	StatusCode() int
	Body() any
}

// Handler 处理函数
type Handler func(ctx context.Context, req Request) (Response, error)

// Interceptor 中间件拦截器接口
type Interceptor interface {
	Intercept(ctx context.Context, req Request, handler Handler) (Response, error)
	Name() string
}

// Chain 中间件链
type Chain []Interceptor

// NewChain 创建中间件链
func NewChain(interceptors ...Interceptor) Chain {
	return Chain(interceptors)
}

// Handle 通过链处理请求
func (c Chain) Handle(ctx context.Context, req Request, final Handler) (Response, error) {
	handler := final
	for i := len(c) - 1; i >= 0; i-- {
		mw := c[i]
		h := handler
		handler = func(ctx context.Context, req Request) (Response, error) {
			return mw.Intercept(ctx, req, h)
		}
	}
	return handler(ctx, req)
}

// Append 追加中间件，返回新链
func (c Chain) Append(m Interceptor) Chain {
	return append(c, m)
}
