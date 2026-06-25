package components

import (
	"github.com/go-zeus/zeus/trace"
)

// TraceComponent 链路追踪组件适配器
type TraceComponent struct {
	tracer trace.Tracer
}

// NewTraceComponent 创建链路追踪组件
// 使用传入的 trace.Tracer 实例
func NewTraceComponent(tracer trace.Tracer) *TraceComponent {
	return &TraceComponent{tracer: tracer}
}

func (t *TraceComponent) Name() string      { return "trace" }
func (t *TraceComponent) Depends() []string { return nil }

func (t *TraceComponent) Provide(ctx Context) (any, error) {
	return t.tracer, nil
}

func (t *TraceComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop: func(ctx Context) error {
			if t.tracer != nil {
				return t.tracer.Close()
			}
			return nil
		},
	}
}
