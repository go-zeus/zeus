package components

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/server"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

// middlewareApplicable server 实现可选的中间件注入接口（鸭子类型）
//
// httpServer.ApplyMiddleware / plugins/server/grpc.ApplyMiddleware 等实现都满足此接口。
// ServerComponent.OnStart 通过类型断言识别，并把容器收集到的中间件链注入到每个 server。
type middlewareApplicable interface {
	ApplyMiddleware(chain middleware.Chain)
}

// isGracefulShutdownErr 判断错误是否属于优雅关闭的正常返回
// net/http 在 Shutdown 后 Serve 返回 ErrServerClosed；其他实现可能有类似语义
func isGracefulShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, http.ErrServerClosed) {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

// ServerComponent 服务端组件适配器
//
// 支持单 server 和多 server 场景：
//   - 单 server：NewServerComponent(http.NewHTTP(http.Port(9001)))
//   - 多 server：NewServerComponent(http.NewHTTP(...), grpc.NewGRPC(...))
//   - 默认 HTTP：NewServerComponent()（不传任何 server，懒构造默认 HTTP server 监听 :8080）
//
// Provide 始终返回 []server.Server，方便 ServiceComponent 为每个 server 生成一个 Instance。
// OnStart 阶段会：
//  1. 自动收集容器中所有 MiddlewareComponent 注册的 Interceptor（按拓扑序）
//     并注入到每个支持 ApplyMiddleware 的 server
//  2. 异步启动所有 server（Server.Start 通常是阻塞的）
//
// OnStop 逆序关闭所有 server。
type ServerComponent struct {
	servers []server.Server

	// server 运行时控制：startCtx 用于通知 server 优雅关闭，wg 等待所有 server.Serve 返回
	startCtx    context.Context
	startCancel context.CancelFunc
	wg          sync.WaitGroup
	startErr    error
	startOnce   sync.Once
}

// NewServerComponent 创建服务端组件
//
// 调用方传入已构造的 server 实例。若不传任何 server，Provide 时会创建默认 HTTP server。
func NewServerComponent(servers ...server.Server) *ServerComponent {
	return &ServerComponent{servers: servers}
}

func (s *ServerComponent) Name() string      { return "server" }
func (s *ServerComponent) Depends() []string { return nil }

func (s *ServerComponent) Provide(ctx Context) (any, error) {
	// 若未显式传入 server，懒构造默认 HTTP server（向后兼容 NewServerComponent()）
	if len(s.servers) == 0 {
		s.servers = []server.Server{httpdriver.NewHTTP()}
	}
	return s.servers, nil
}

func (s *ServerComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			// 1. 收集容器中所有 MiddlewareComponent 注册的 Interceptor（按拓扑序）
			//    并把它们注入到每个支持 ApplyMiddleware 的 server
			applyMiddlewares(ctx, s.servers)

			// 2. 准备 server 生命周期 context：随容器 ctx 取消（应用退出时）
			s.startCtx, s.startCancel = context.WithCancel(ctx)
			// 并发启动所有 server；任一 server 异常退出时记录首个错误
			for _, srv := range s.servers {
				s.wg.Add(1)
				go func(srv server.Server) {
					defer s.wg.Done()
					if err := srv.Start(s.startCtx); err != nil && !isGracefulShutdownErr(err) {
						s.startOnce.Do(func() {
							s.startErr = err
							log.Error("server %s exited with error: %v", srv.Protocol(), err)
						})
					}
				}(srv)
			}
			return nil
		},
		OnStop: func(ctx Context) error {
			// 通知所有 server 进入优雅关闭流程
			if s.startCancel != nil {
				s.startCancel()
			}
			// 逆序调用 Stop，符合"后启动先停止"惯例
			stopCtx, cancel := context.WithTimeout(ctx, defaultStopTimeout)
			defer cancel()
			var firstErr error
			for i := len(s.servers) - 1; i >= 0; i-- {
				if err := s.servers[i].Stop(stopCtx); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			// 等待所有 server.Serve 返回，避免 goroutine 泄漏
			s.wg.Wait()
			// 若 server 运行期出错，优先返回该错误
			if s.startErr != nil {
				return s.startErr
			}
			return firstErr
		},
	}
}

// applyMiddlewares 收集容器中所有 middleware.Interceptor（按拓扑序），组成 Chain，
// 并通过 ApplyMiddleware 注入到每个支持该接口的 server。
//
// 行为说明：
//   - 没有中间件时为 no-op（chain 为空，httpServer.ApplyMiddleware 会直接 return）
//   - 不实现 middlewareApplicable 的 server 被忽略（兼容无中间件接口的 server 实现）
//   - OnStart 调用时机保证：此时所有 MiddlewareComponent.Provide 都已完成
func applyMiddlewares(ctx Context, servers []server.Server) {
	if len(servers) == 0 {
		return
	}
	interceptors, err := AllByType[middleware.Interceptor](ctx)
	if err != nil || len(interceptors) == 0 {
		return
	}
	chain := middleware.NewChain(interceptors...)
	for _, srv := range servers {
		if app, ok := srv.(middlewareApplicable); ok {
			app.ApplyMiddleware(chain)
		}
	}
}
