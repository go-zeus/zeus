package noop

import (
	"github.com/go-zeus/zeus/metrics"
)

type noopMeter struct{}
type noopCounter struct{}
type noopHistogram struct{}
type noopGauge struct{}

// New 创建 noop Meter
func New() metrics.Meter { return &noopMeter{} }

// IsNoop 判断 meter 是否为 noop 实现。
//
// 供埋点热路径（如 cache/memory）在 noop 时跳过 labels map 构造开销：
// 调用方在构造期探测一次，存 bool 标志，避免每次操作分配 labels map。
func IsNoop(m metrics.Meter) bool {
	_, ok := m.(*noopMeter)
	return ok
}

func (n *noopMeter) Counter(_ string, _ map[string]string) metrics.Counter { return &noopCounter{} }
func (n *noopMeter) Histogram(_ string, _ map[string]string) metrics.Histogram {
	return &noopHistogram{}
}
func (n *noopMeter) Gauge(_ string, _ map[string]string) metrics.Gauge { return &noopGauge{} }
func (n *noopMeter) Close() error                                      { return nil }

func (n *noopCounter) Inc()                {}
func (n *noopCounter) Add(_ float64)       {}
func (n *noopHistogram) Observe(_ float64) {}
func (n *noopGauge) Set(_ float64)         {}
func (n *noopGauge) Inc()                  {}
func (n *noopGauge) Dec()                  {}
