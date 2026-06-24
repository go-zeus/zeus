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
