package components

import (
	"github.com/go-zeus/zeus/middleware"
)

// MiddlewareComponent 中间件组件适配器
//
// 行为说明：
//   - Provide 时把 middleware.Interceptor 注册到装配上下文
//   - ServerComponent.OnStart 自动从装配上下文收集所有 MiddlewareComponent 注册的
//     Interceptor，组成 middleware.Chain，并通过 ApplyMiddleware 注入到每个支持
//     该接口的 server（当前：server/http）
//   - 多个 MiddlewareComponent 的生效顺序按 Container 拓扑排序（同层按字典序），
//     字典序由 "middleware_<interceptor.Name()>" 决定。需要严格控制顺序的场景，
//     请使用 middleware.NewChain + httpdriver.ChainHandler 显式装配。
//
// 用法示例（L3 装配模式，中间件自动应用）：
//
//	app := components.NewApp(
//	    components.NewMiddlewareComponent(recovery.New()),
//	    components.NewMiddlewareComponent(tracing.New()),
//	    components.NewServerComponent(http.NewHTTP()),
//	)
type MiddlewareComponent struct {
	name string
	mw   middleware.Interceptor
}

// NewMiddlewareComponent 创建中间件组件
func NewMiddlewareComponent(mw middleware.Interceptor) *MiddlewareComponent {
	return &MiddlewareComponent{name: mw.Name(), mw: mw}
}

func (m *MiddlewareComponent) Name() string      { return "middleware_" + m.name }
func (m *MiddlewareComponent) Depends() []string { return nil }

func (m *MiddlewareComponent) Provide(ctx Context) (any, error) {
	return m.mw, nil
}

func (m *MiddlewareComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop:  func(ctx Context) error { return nil },
	}
}
