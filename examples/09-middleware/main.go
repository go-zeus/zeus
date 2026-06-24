// Example: middleware
//
// 演示 middleware.Chain 通用接口的独立用法（不依赖 HTTP server）：
//   - 构造 Chain：recovery + timeout
//   - 显式调用 chain.Handle 执行链
//
// 这是底层接口演示，适合：
//   - 测试中间件本身的语义
//   - 自定义非 HTTP 协议的中间件链
//   - 学习中间件拦截模式
//
// 实际 HTTP 服务推荐两种装配方式：
//
//	// 方式 1：L3 自动应用（简单场景）
//	components.NewApp(
//	    components.NewMiddlewareComponent(recovery.New()),
//	    components.NewMiddlewareComponent(timeout.New(2*time.Second)),
//	    components.NewServerComponent(http.NewHTTP(http.Mux(handler))),
//	)
//	// ServerComponent.OnStart 自动收集所有 Interceptor 按字典序应用
//
//	// 方式 2：手动 ChainHandler（严格顺序控制）
//	chain := middleware.NewChain(recovery.New(), timeout.New(2*time.Second))
//	srv := http.NewHTTP(http.Mux(http.ChainHandler(handler, chain)))
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/middleware/timeout"
)

// mockRequest 模拟请求
type mockRequest struct{}

func (r *mockRequest) Method() string           { return "GET" }
func (r *mockRequest) Path() string             { return "/api/test" }
func (r *mockRequest) Header(key string) string { return "" }
func (r *mockRequest) Body() any                { return nil }

// mockResponse 模拟响应
type mockResponse struct {
	code int
	body string
}

func (r *mockResponse) StatusCode() int { return r.code }
func (r *mockResponse) Body() any       { return r.body }

func main() {
	// 构建中间件链：recovery → timeout → 业务处理
	chain := middleware.NewChain(
		recovery.New(),
		timeout.New(2*time.Second),
	)

	// 正常请求
	resp, err := chain.Handle(context.Background(), &mockRequest{}, func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		return &mockResponse{code: 200, body: "ok"}, nil
	})
	fmt.Printf("normal: status=%d body=%v err=%v\n", resp.StatusCode(), resp.Body(), err)

	// panic 请求（被 recovery 捕获）
	resp, err = chain.Handle(context.Background(), &mockRequest{}, func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		panic("something went wrong")
	})
	fmt.Printf("panic: resp=%v err=%v\n", resp, err)
}
