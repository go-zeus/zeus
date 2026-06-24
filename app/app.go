package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-zeus/zeus/server"
	_ "github.com/go-zeus/zeus/utils/banner"
	"github.com/go-zeus/zeus/utils/errgroup"
)

// App 应用接口
// 推荐使用 components.NewApp（自动装配模式），本包为兼容旧 API 保留
type App interface {
	Run(close <-chan struct{}) error
}

// New 创建应用（L4 手动装配入口）
//
// 必须通过 WithServer 显式传入至少一个 server：
//
//	a := app.New(app.WithServer(http.NewHTTP(http.Port(8080))))
//	a.Run(make(chan struct{}))
//
// 对新代码推荐使用 L1 入口 app.Run(cfg, handler)，
// 内部自动按 handler 类型选择 server 实现。
//
// 不传 server 时返回的 App 在 Run 时会返回 error，
// 避免依赖 server.DefaultServer 全局变量（已废弃）。
func New(opts ...Option) App {
	a := &app{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

type Option func(s *app)

// WithServer 添加服务器
func WithServer(srv server.Server) Option {
	return func(s *app) {
		s.servers = append(s.servers, srv)
	}
}

type app struct {
	servers []server.Server
}

func (a *app) Run(close <-chan struct{}) error {
	// 防御 DefaultServer 为 nil 的情况（未 import 具体 server 实现）
	if len(a.servers) == 0 || a.servers[0] == nil {
		return errors.New("app: no server configured (import server/http or pass WithServer)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)

	// 启动所有 server
	for _, s := range a.servers {
		s := s
		eg.Go(func() error {
			return s.Start(ctx)
		})
	}

	// 监听退出信号
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	eg.Go(func() error {
		select {
		case <-close:
			cancel()
			return nil
		case sig := <-ch:
			cancel()
			return errors.New("get signal " + sig.String() + ", application will shutdown\n")
		}
	})

	runErr := eg.Wait()

	// 优雅关闭：收集首个 stop error
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	var stopErr error
	for _, s := range a.servers {
		if s == nil {
			continue
		}
		if err := s.Stop(stopCtx); err != nil && stopErr == nil {
			stopErr = fmt.Errorf("app: server stop failed: %w", err)
		}
	}
	// 优先返回 run 错误，stop 错误次之
	if runErr != nil {
		return runErr
	}
	return stopErr
}
