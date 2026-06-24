package components

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const defaultStopTimeout = 10 * time.Second

// App 基于 Container 的应用
// 提供声明式组件组装 + 信号监听 + 优雅关闭
type App struct {
	container *Container
	timeout   time.Duration
}

// AppOption 应用选项
type AppOption func(*App)

// NewApp 创建应用
// 接受 Component 和 AppOption 混合参数
// 注册失败时 panic（名称冲突或空名称属于编程错误）
func NewApp(comps ...any) *App {
	c := NewContainer()
	var opts []AppOption
	for _, comp := range comps {
		switch v := comp.(type) {
		case Component:
			if err := c.Register(v); err != nil {
				panic("components: " + err.Error())
			}
		case AppOption:
			opts = append(opts, v)
		default:
			panic("components: NewApp accepts Component or AppOption, got " + fmt.Sprintf("%T", v))
		}
	}
	app := &App{
		container: c,
		timeout:   defaultStopTimeout,
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// WithStopTimeout 设置优雅关闭超时时间
func WithStopTimeout(d time.Duration) AppOption {
	return func(a *App) { a.timeout = d }
}

// Run 启动应用，阻塞直到收到退出信号
func (a *App) Run() error {
	ctx := context.Background()

	// 启动所有组件
	if err := a.container.Start(ctx); err != nil {
		return err
	}

	// 监听退出信号
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	<-ch

	// 优雅关闭
	stopCtx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	if err := a.container.Stop(stopCtx); err != nil {
		return err
	}

	return nil
}

// RunWithContext 启动应用，支持 context 取消
func (a *App) RunWithContext(ctx context.Context) error {
	// 启动所有组件
	if err := a.container.Start(ctx); err != nil {
		return err
	}

	// 监听退出信号或 context 取消
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	select {
	case <-ctx.Done():
	case <-ch:
	}

	// 优雅关闭
	stopCtx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	return a.container.Stop(stopCtx)
}

// Container 获取底层容器
func (a *App) Container() *Container {
	return a.container
}

// Get 按名称获取组件实例
func (a *App) Get(name string) (any, bool) {
	return a.container.Get(name)
}

// GetByType 按类型从 App 获取组件实例
func GetByTypeFromApp[T any](a *App) (T, bool) {
	return GetByType[T](a.container)
}
