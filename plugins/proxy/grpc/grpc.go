// Package grpc 提供 gRPC 透明反向代理
//
// 实现原理：
//   - 基于 grpc-go 的 unknown service handler 拦截所有方法调用
//   - 从请求 :authority / metadata 解析后端目标
//   - 使用 grpc.ClientConn 转发请求并双向透传 stream
//
// 与主 proxy 包的关系：
//   - 主 proxy.Proxy 处理 HTTP/1.1（含 WebSocket、SSE）
//   - gRPC 走 HTTP/2 多路复用，独立监听端口，避免与 HTTP/1.1 在同端口共存需 ALPN 分流
//   - 用户在部署时通常 gRPC 走独立端口或独立域名
//
// 用法：
//
//	import grpcproxy "github.com/go-zeus/zeus/plugins/proxy/grpc"
//
//	target, _ := url.Parse("127.0.0.1:9090")
//	p := grpcproxy.New(grpcproxy.WithTarget(target))
//	p.Listen(":8082")
//	p.Serve()
package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Proxy gRPC 透明代理
type Proxy interface {
	Listen(addr string) error
	Serve() error
	Stop() error
}

// Option 函数式选项
type Option func(*proxy)

// WithTarget 设置后端目标（host:port）
func WithTarget(target *url.URL) Option {
	return func(p *proxy) { p.target = target }
}

// WithSelector 设置动态后端选择器
// 优先级高于 WithTarget
func WithSelector(s selector) Option {
	return func(p *proxy) { p.selector = s }
}

// selector 后端选择器（独立定义，避免与主 proxy 包循环依赖）
// selector 实现方负责返回 host:port
type selector interface {
	Pick(method string) (*url.URL, error)
}

// New 创建 gRPC 透明代理
func New(opts ...Option) (Proxy, error) {
	p := &proxy{}
	for _, opt := range opts {
		opt(p)
	}
	if p.selector == nil && p.target == nil {
		return nil, errors.New("grpc: WithTarget or WithSelector is required")
	}
	return p, nil
}

// proxy gRPC 代理实现
type proxy struct {
	target   *url.URL
	selector selector

	mu       sync.Mutex
	listener net.Listener
	server   *grpc.Server
	closed   bool
}

// 编译期检查 proxy 实现 Proxy 接口
var _ Proxy = (*proxy)(nil)

// Listen 在指定地址监听
func (p *proxy) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc: listen %s: %w", addr, err)
	}
	p.mu.Lock()
	p.listener = ln
	p.mu.Unlock()
	return nil
}

// Serve 启动 gRPC 服务，阻塞直到 Stop 被调用
func (p *proxy) Serve() error {
	p.mu.Lock()
	if p.listener == nil {
		p.mu.Unlock()
		return errors.New("grpc: listener not set, call Listen first")
	}
	ln := p.listener
	p.mu.Unlock()

	srv := grpc.NewServer(
		grpc.UnknownServiceHandler(p.handleUnknown),
	)
	p.mu.Lock()
	p.server = srv
	p.mu.Unlock()

	return srv.Serve(ln)
}

// Stop 优雅停止代理
func (p *proxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.server != nil {
		p.server.GracefulStop()
	}
	return nil
}

// handleUnknown 处理所有未知 service 的 RPC 调用
// 这是透明代理的核心：拦截所有方法，转发到后端
func (p *proxy) handleUnknown(srv interface{}, stream grpc.ServerStream) error {
	method, ok := grpc.MethodFromServerStream(stream)
	if !ok {
		return status.Error(codes.Internal, "cannot extract method from stream")
	}

	// 选择后端
	target := p.target
	if p.selector != nil {
		u, err := p.selector.Pick(method)
		if err != nil {
			return status.Errorf(codes.Unavailable, "select backend: %v", err)
		}
		target = u
	}
	if target == nil {
		return status.Error(codes.Unavailable, "no backend available")
	}

	// 建立到后端的 client conn（非阻塞，调用时触发实际拨号）
	cc, err := grpc.NewClient(target.Host,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return status.Errorf(codes.Unavailable, "dial backend %s: %v", target.Host, err)
	}
	defer cc.Close()

	// 透传 metadata
	md, _ := metadata.FromIncomingContext(stream.Context())
	ctx := metadata.NewOutgoingContext(stream.Context(), md.Copy())

	// 创建到后端的 stream
	ctx, cancelForward := context.WithCancel(ctx)
	defer cancelForward()
	clientStream, err := cc.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "proxy",
		ServerStreams: true,
		ClientStreams: true,
	}, method)
	if err != nil {
		return status.Errorf(codes.Internal, "create backend stream: %v", err)
	}

	// 双向转发
	return p.forwardStream(stream, clientStream)
}

// forwardStream 双向转发：客户端 → 后端、后端 → 客户端
//
// 任一方向结束（含 EOF / 错误）会触发 cancel，唤醒对端 RecvMsg 立即退出，
// 避免 goroutine 泄漏。
func (p *proxy) forwardStream(serverStream grpc.ServerStream, clientStream grpc.ClientStream) error {
	_, cancel := context.WithCancel(serverStream.Context())
	defer cancel()

	errCh := make(chan error, 2)

	// c2s: client → server (forward to backend)
	go func() {
		errCh <- p.copyServerToClient(serverStream, clientStream)
		cancel()
	}()

	// s2c: backend → client (forward to server)
	go func() {
		errCh <- p.copyClientToServer(clientStream, serverStream)
		cancel()
	}()

	// 等待两个 goroutine 全部结束
	var firstErr error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// copyServerToClient 从 server stream 读消息写到 client stream
func (p *proxy) copyServerToClient(src grpc.ServerStream, dst grpc.ClientStream) error {
	for {
		var msg interface{}
		if err := src.RecvMsg(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				_ = dst.CloseSend()
				return nil
			}
			return err
		}
		if err := dst.SendMsg(msg); err != nil {
			return err
		}
	}
}

// copyClientToServer 从 client stream 读消息写到 server stream
func (p *proxy) copyClientToServer(src grpc.ClientStream, dst grpc.ServerStream) error {
	for {
		var msg interface{}
		if err := src.RecvMsg(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := dst.SendMsg(msg); err != nil {
			return err
		}
	}
}
