package components

// Lifecycle 组件生命周期钩子
type Lifecycle struct {
	OnStart func(ctx Context) error
	OnStop  func(ctx Context) error
}

// Component 组件接口
type Component interface {
	// Name 组件名称，全局唯一
	Name() string

	// Depends 声明依赖的其他组件名称
	// 框架据此推导启动顺序
	Depends() []string

	// Provide 提供实例，供其他组件注入
	// 返回 (实例, 错误)，错误不为 nil 时终止启动
	Provide(ctx Context) (any, error)

	// Lifecycle 组件生命周期钩子
	Lifecycle() Lifecycle
}
