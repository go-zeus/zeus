package noop_test

import (
	"testing"

	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/metrics/noop"
)

func TestNew(t *testing.T) {
	m := noop.New()
	if m == nil {
		t.Fatal("expected non-nil Meter")
	}
}

func TestCounter(t *testing.T) {
	m := noop.New()
	c := m.Counter("test_counter", map[string]string{"method": "GET"})
	c.Inc()
	c.Add(1.0)
}

func TestHistogram(t *testing.T) {
	m := noop.New()
	h := m.Histogram("test_histogram", map[string]string{"path": "/api"})
	h.Observe(1.0)
}

func TestGauge(t *testing.T) {
	m := noop.New()
	g := m.Gauge("test_gauge", map[string]string{"status": "ok"})
	g.Set(1.0)
	g.Inc()
	g.Dec()
}

func TestClose(t *testing.T) {
	m := noop.New()
	if err := m.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeterInterface(t *testing.T) {
	// 验证 noop.New() 返回值满足 metrics.Meter 接口
	var _ metrics.Meter = noop.New()
}
