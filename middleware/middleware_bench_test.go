package middleware

import (
	"context"
	"testing"
)

// middleware.Chain.Handle 是每个 HTTP/gRPC 请求必经的热路径。
// 本 bench 量化：
//   - 链构建开销（NewChain，仅一次性）
//   - 链执行开销（Handle，每请求一次）—— 真正的性能敏感点
//   - 不同链深度下的每层边际成本（1 / 5 / 10 拦截器）
//
// 典型链深度：recovery + requestID + accesslog + tracing + metrics = 5 层
// （见 CLAUDE.md 可观测性章节），故 BenchmarkHandle_5 是最重要的回归基线。

// noopInterceptor 透传拦截器：仅测量链调度开销，不含业务逻辑
type noopInterceptor struct{}

func (noopInterceptor) Intercept(ctx context.Context, req Request, h Handler) (Response, error) {
	return h(ctx, req)
}
func (noopInterceptor) Name() string { return "noop" }

// benchRequest 最小 Request 实现（避免引入具体协议依赖）
type benchRequest struct{}

func (benchRequest) Method() string      { return "GET" }
func (benchRequest) Path() string        { return "/bench" }
func (benchRequest) Header(string) string { return "" }
func (benchRequest) Body() any           { return nil }

// benchResponse 最小 Response 实现
type benchResponse struct{}

func (benchResponse) StatusCode() int { return 200 }
func (benchResponse) Body() any       { return nil }

// makeChain 构造 n 层 noop 链
func makeChain(n int) Chain {
	ints := make([]Interceptor, n)
	for i := range ints {
		ints[i] = noopInterceptor{}
	}
	return NewChain(ints...)
}

// BenchmarkNewChain 链构建开销（一次性，参考用）
func BenchmarkNewChain(b *testing.B) {
	ints := []Interceptor{noopInterceptor{}, noopInterceptor{}, noopInterceptor{}, noopInterceptor{}, noopInterceptor{}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewChain(ints...)
	}
}

// BenchmarkHandle_1 1 层链执行
func BenchmarkHandle_1(b *testing.B) { benchHandleN(b, 1) }

// BenchmarkHandle_5 5 层链执行（典型可观测性链，最重要基线）
func BenchmarkHandle_5(b *testing.B) { benchHandleN(b, 5) }

// BenchmarkHandle_10 10 层链执行（量化每层边际成本）
func BenchmarkHandle_10(b *testing.B) { benchHandleN(b, 10) }

func benchHandleN(b *testing.B, n int) {
	chain := makeChain(n)
	ctx := context.Background()
	req := benchRequest{}
	final := func(ctx context.Context, req Request) (Response, error) {
		return benchResponse{}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Handle(ctx, req, final)
	}
}
