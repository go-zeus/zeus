package components

import (
	"github.com/go-zeus/zeus/metrics"
)

// MetricsComponent 指标组件适配器
type MetricsComponent struct {
	meter metrics.Meter
}

// NewMetricsComponent 创建指标组件
// 使用传入的 metrics.Meter 实例
func NewMetricsComponent(meter metrics.Meter) *MetricsComponent {
	return &MetricsComponent{meter: meter}
}

func (m *MetricsComponent) Name() string      { return "metrics" }
func (m *MetricsComponent) Depends() []string { return nil }

func (m *MetricsComponent) Provide(ctx Context) (any, error) {
	return m.meter, nil
}

func (m *MetricsComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop: func(ctx Context) error {
			if m.meter != nil {
				return m.meter.Close()
			}
			return nil
		},
	}
}
