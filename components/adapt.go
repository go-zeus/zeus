package components

// Adapt 将任意实例快速包装为 Component
// 适用于不需要生命周期管理的简单组件
func Adapt(name string, value any, depends ...string) Component {
	return &adaptedComponent{
		name:    name,
		depends: depends,
		value:   value,
	}
}

type adaptedComponent struct {
	name    string
	depends []string
	value   any
}

func (a *adaptedComponent) Name() string                     { return a.name }
func (a *adaptedComponent) Depends() []string                { return a.depends }
func (a *adaptedComponent) Provide(ctx Context) (any, error) { return a.value, nil }
func (a *adaptedComponent) Lifecycle() Lifecycle             { return Lifecycle{} }
