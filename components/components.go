package components

var cm = &components{data: make(map[string]*Component)}

type components struct {
	data map[string]*Component
}

type Component struct {
	Name    string                              //组件名称
	builder func(opts ...any) (Instance, error) //组件初始化函数
}

func Register(name string, builder func(opts ...any) (Instance, error)) {
	if name == "" {
		panic("组件名称不能为空")
	}
	if builder == nil {
		panic("组件实例化方法不能为空")
	}
	comp := &Component{
		Name:    name,
		builder: builder,
	}
	cm.data[comp.Name] = comp
}

func Get(kind string) *Component {
	return cm.data[kind]
}
