// Package grpc 提供基于 google.golang.org/grpc 的 gRPC 服务器实现。
//
// 特性：
//   - 默认 UnaryServerInterceptor：从 metadata["x-zeus-cluster"] 提取集群标记注入 context
//   - 支持 Register(func(*grpc.Server)) 注册业务 Service
//   - 支持自定义 UnaryServerInterceptor 链
//   - graceful shutdown（监听 ctx 取消）
//
// 用法 1（L3 装配）：
//
//	import grpcserver "github.com/go-zeus/zeus/plugins/server/grpc"
//
//	s := grpcserver.NewGRPC(
//	    grpcserver.Port(9090),
//	    grpcserver.Register(func(s *grpc.Server) {
//	        mypb.RegisterMyServiceServer(s, &myServiceImpl{})
//	    }),
//	)
//	components.NewApp(components.NewServerComponent(s))
//
// 用法 2（L1 入口，导入即用）：
//
//	import (
//	    _ "github.com/go-zeus/zeus/plugins/server/grpc"
//	    "github.com/go-zeus/zeus/app"
//	)
//
//	gs := grpc.NewServer()  // 用户已经构造并注册了 service
//	app.Run(&app.Config{Port: 9090}, gs)
//
// 本包通过 init() 调用 app.RegisterServerResolver，把 *grpc.Server 类型嗅探能力
// 注册到 L1 入口；主包零依赖 grpc。
package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/go-zeus/zeus/app"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/server"
	"google.golang.org/grpc"
	grpcmeta "google.golang.org/grpc/metadata"
)

// init 把 *grpc.Server 嗅探器注册到 L1 入口（副作用 import 模式）。
// 用户 _ "github.com/go-zeus/zeus/plugins/server/grpc" 后即可在 app.Run 中
// 直接传入 *grpc.Server。
func init() {
	app.RegisterServerResolver(func(handler any, cfg *app.Config) (server.Server, error) {
		gs, ok := handler.(*grpc.Server)
		if !ok {
			return nil, nil // 不匹配，让下一个 resolver 试
		}
		opts := []Option{Port(cfg.Port)}
		if cfg.IP != "" {
			opts = append(opts, Ip(cfg.IP))
		}
		return FromGRPC(gs, opts...), nil
	})
}

const (
	ProtocolName = "grpc"
	DefaultPort  = 9090
)

// Option 函数式选项
type Option func(*grpcServer)

// Port 监听端口（默认 9090）
func Port(p int) Option {
	return func(s *grpcServer) { s.port = p }
}

// Ip 监听 IP（默认空，监听所有接口）
func Ip(ip string) Option {
	return func(s *grpcServer) { s.ip = ip }
}

// Register 注册业务 ServiceDesc 的回调。
// 可多次调用，按顺序执行。
func Register(fn func(*grpc.Server)) Option {
	return func(s *grpcServer) {
		s.registers = append(s.registers, fn)
	}
}

// Interceptor 追加自定义 UnaryServerInterceptor。
// 多个 interceptor 按追加顺序执行；clusterInterceptor 始终最先执行。
func Interceptor(i ...grpc.UnaryServerInterceptor) Option {
	return func(s *grpcServer) {
		s.userInterceptors = append(s.userInterceptors, i...)
	}
}

// WithoutAutoClustering 关闭默认的集群路由自动注入（X-Zeus-Cluster metadata → context）。
func WithoutAutoClustering() Option {
	return func(s *grpcServer) { s.autoClustering = false }
}

// grpcServer gRPC 服务器实现
type grpcServer struct {
	ip               string
	port             int
	registers        []func(*grpc.Server)
	userInterceptors []grpc.UnaryServerInterceptor
	autoClustering   bool

	// ownerProvided 标记 *grpc.Server 是否由用户提供。
	// true 时 Start 跳过 grpc.NewServer 和 registers 调用，
	// 直接复用用户已经构造好的 server（含已注册的 service / interceptor）。
	ownerProvided bool

	mu       sync.Mutex
	listener net.Listener
	server   *grpc.Server
	closed   bool
}

// 编译期检查 grpcServer 实现 server.Server
var _ server.Server = (*grpcServer)(nil)

// NewGRPC 创建 gRPC 服务器。
// 默认行为：
//   - 监听 :9090
//   - 自动从 metadata["x-zeus-cluster"] 提取集群标记注入 context
func NewGRPC(opts ...Option) server.Server {
	s := &grpcServer{
		port:           DefaultPort,
		autoClustering: true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// FromGRPC 包装用户已经构造好的 *grpc.Server（已经注册了 service / interceptor），
// 让它适配 server.Server 接口，可挂到 components.ServerComponent 或通过 app.Run L1 入口使用。
//
// 与 NewGRPC 的区别：
//   - NewGRPC：server 内部构造 *grpc.Server，用户通过 Register 回调注册 service
//   - FromGRPC：用户提供已构造好的 *grpc.Server，server 直接复用（含所有已注册的 service）
//
// 适合 L1 入口：
//
//	gs := grpc.NewServer()
//	mypb.RegisterMyServiceServer(gs, &myImpl{})
//	app.Run(&app.Config{Port: 9090}, gs)  // 需 import _ ".../plugins/server/grpc"
//
// autoClustering 在 FromGRPC 路径下默认关闭（用户自己的 *grpc.Server 已经有自己的 interceptor 链）。
func FromGRPC(srv *grpc.Server, opts ...Option) server.Server {
	s := &grpcServer{
		port:           DefaultPort,
		autoClustering: false, // 用户 *grpc.Server 已经有自己的链，不再叠加
		server:         srv,
		ownerProvided:  true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *grpcServer) Protocol() string { return ProtocolName }

func (s *grpcServer) Endpoint() string {
	return fmt.Sprintf("%s:%d", s.ip, s.port)
}

// Start 启动 gRPC 服务，阻塞直到 ctx 取消或 listener 关闭
func (s *grpcServer) Start(ctx context.Context) error {
	log.Info("server start is %s", s.Endpoint())

	ln, err := net.Listen("tcp", s.Endpoint())
	if err != nil {
		return fmt.Errorf("grpc: listen %s: %w", s.Endpoint(), err)
	}

	s.mu.Lock()
	s.listener = ln
	srv := s.server
	if !s.ownerProvided {
		// 构造拦截器链：clusterInterceptor 在最前
		chain := make([]grpc.UnaryServerInterceptor, 0, len(s.userInterceptors)+1)
		if s.autoClustering {
			chain = append(chain, clusterInterceptor)
		}
		chain = append(chain, s.userInterceptors...)
		s.server = grpc.NewServer(grpc.ChainUnaryInterceptor(chain...))
		srv = s.server
	}
	s.mu.Unlock()

	if !s.ownerProvided {
		// 执行用户注册（用户 *grpc.Server 路径下 service 已经注册，跳过）
		for _, reg := range s.registers {
			reg(srv)
		}
	}

	// 监听 ctx 取消以优雅关闭
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// GracefulStop 内部自带 active RPC 等待逻辑，
			// 这里不再叠加 timeout context（GracefulStop 不接受 ctx）。
			// 上层 App.Run 的 10s stopCtx 通过 ctx.Done() 触发本路径。
			srv.GracefulStop()
		case <-done:
		}
	}()

	err = srv.Serve(ln)
	close(done)
	if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("grpc: serve %s: %w", s.Endpoint(), err)
	}
	return nil
}

// Stop 优雅停止 gRPC 服务
func (s *grpcServer) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.server != nil {
		s.server.GracefulStop()
	}
	return nil
}

// clusterInterceptor 从 metadata 中提取集群标记和 W3C baggage，注入 ctx。
//
// 行为：
//   - 解析 metadata["x-zeus-cluster"] → routing.WithCluster 注入 ctx
//   - 解析 metadata["baggage"]（W3C baggage）→ propagation.ExtractMetadataMulti 注入 ctx
//   - 缺失时回退到 routing.Default
//
// 下游业务 handler / client / log / trace 可通过：
//   - routing.FromContext(ctx) 读 cluster
//   - propagation.Get(ctx, "tenant.id") 读用户自定义 K-V
func clusterInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	c := routing.Default
	if md, ok := grpcmeta.FromIncomingContext(ctx); ok {
		if vals := md.Get(routing.MetadataCluster); len(vals) > 0 && vals[0] != "" {
			c = vals[0]
		}
	}
	ctx = routing.WithCluster(ctx, c)
	// 自动 extract baggage（用户自定义 K-V）
	if md, ok := grpcmeta.FromIncomingContext(ctx); ok {
		ctx = propagation.ExtractMetadataMulti(ctx, md)
	}
	return handler(ctx, req)
}
