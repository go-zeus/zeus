// Package proxy 反向代理网关
//
// 提供统一的反向代理入口，自动嗅探协议分流：
//   - HTTP/HTTPS：标准反向代理，注入 X-Forwarded-For / X-Real-IP / X-Request-ID
//   - WebSocket：Hijack 客户端连接，raw io.Copy 双向透传（nginx 方式）
//   - SSE：串行 read-write-flush，保证事件顺序
//
// gRPC 代理因 HTTP/2 多路复用特性走独立 plugin 模块（plugins/proxy/grpc），
// 与 HTTP/1.1 同端口共存需复杂 ALPN 分流，违背 KISS 原则。
//
// 用法：
//
//	target, _ := url.Parse("http://127.0.0.1:9000")
//	p := proxy.New(proxy.WithSelector(proxy.NewStaticSelector(target)))
//	http.ListenAndServe(":8081", p)
package proxy

import (
	"fmt"
	"net/http"
	"net/url"
)

// Selector 后端选择器
// 抽象静态 URL 与动态服务发现两种模式，支持运行时切换
type Selector interface {
	// Pick 根据请求选择后端目标 URL
	// 实现应线程安全，会被并发调用
	Pick(r *http.Request) (*url.URL, error)
}

// Director 请求重写扩展点
// 与 httputil.ReverseProxy.Director 签名兼容，可组合使用
type Director func(target *url.URL, req *http.Request)

// ResponseRewriter 响应改写扩展点
type ResponseRewriter func(resp *http.Response) error

// ErrorHandler 错误处理扩展点
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// Proxy 反向代理网关
// 实现标准 http.Handler，可作为 http.Server 的 Handler
type Proxy interface {
	http.Handler
}

// Option 函数式选项
type Option func(*proxy)

// WithSelector 设置后端选择器（必填）
// 缺失会在 New 时 panic，属于编程错误
func WithSelector(s Selector) Option {
	return func(p *proxy) { p.selector = s }
}

// WithDirector 自定义请求重写
// 与默认 X-Forwarded-For/X-Real-IP/X-Request-ID 注入叠加生效
func WithDirector(d Director) Option {
	return func(p *proxy) { p.userDirector = d }
}

// WithResponseRewriter 自定义响应改写
func WithResponseRewriter(r ResponseRewriter) Option {
	return func(p *proxy) { p.responseRewriter = r }
}

// WithErrorHandler 自定义错误处理
// 默认 502 + log
func WithErrorHandler(h ErrorHandler) Option {
	return func(p *proxy) { p.errorHandler = h }
}

// WithTransport 自定义 RoundTripper
// 可注入重试、trace、连接池等
func WithTransport(t http.RoundTripper) Option {
	return func(p *proxy) { p.transport = t }
}

// New 创建反向代理
// 必须提供 WithSelector，否则 panic（编程错误）
func New(opts ...Option) Proxy {
	p := &proxy{
		transport:    http.DefaultTransport,
		errorHandler: defaultErrorHandler,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.selector == nil {
		panic("proxy: WithSelector is required")
	}
	return p
}

// defaultErrorHandler 默认错误处理：502 + 日志
func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(fmt.Sprintf("proxy: %v\n", err)))
	}
}

// proxy 反向代理实现
type proxy struct {
	selector         Selector
	userDirector     Director
	responseRewriter ResponseRewriter
	errorHandler     ErrorHandler
	transport        http.RoundTripper
}

// 编译期检查 proxy 实现 Proxy 接口
var _ Proxy = (*proxy)(nil)

// ServeHTTP 实现 http.Handler，按协议自动分流
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case isWebSocketUpgrade(r):
		p.handleWebSocket(w, r)
	case isSSERequest(r):
		p.handleSSE(w, r)
	default:
		p.handleHTTP(w, r)
	}
}
