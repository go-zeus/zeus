package prometheus

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// newIsolatedMeter 用独立 Registerer 避免污染 DefaultRegisterer
func newIsolatedMeter(opts ...Option) (metrics.Meter, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	opts = append([]Option{WithRegisterer(reg)}, opts...)
	return New(opts...), reg
}

func TestCounter_IncAndAdd(t *testing.T) {
	m, reg := newIsolatedMeter(WithNamespace("zeus"))
	c := m.Counter("requests_total", map[string]string{"method": "GET"})
	c.Inc()
	c.Add(2)

	mf, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(mf) != 1 {
		t.Fatalf("expected 1 metric family, got %d", len(mf))
	}
	if mf[0].GetName() != "zeus_requests_total" {
		t.Fatalf("unexpected name: %s", mf[0].GetName())
	}
	if got := mf[0].Metric[0].Counter.GetValue(); got != 3 {
		t.Fatalf("expected value 3, got %v", got)
	}
}

func TestCounter_LabelsOrderIrrelevant(t *testing.T) {
	m, reg := newIsolatedMeter()
	// 两次传入顺序不同但 keys 相同 → 应该共享 Vec
	c1 := m.Counter("hits", map[string]string{"a": "1", "b": "2"})
	c2 := m.Counter("hits", map[string]string{"b": "2", "a": "1"})
	c1.Inc()
	c2.Inc()

	mf, _ := reg.Gather()
	if len(mf[0].Metric) != 1 {
		t.Fatalf("labels 顺序无关应复用同一个时间序列，got %d series", len(mf[0].Metric))
	}
	if got := mf[0].Metric[0].Counter.GetValue(); got != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

func TestCounter_RepeatedCallIdempotent(t *testing.T) {
	m, _ := newIsolatedMeter()
	for i := 0; i < 100; i++ {
		m.Counter("dup", map[string]string{"k": "v"}).Inc()
	}
	// 不 panic 即说明 AlreadyRegistered 分支生效
}

func TestHistogram_Observe(t *testing.T) {
	m, reg := newIsolatedMeter(WithDefaultBuckets([]float64{1, 5, 10}))
	h := m.Histogram("latency", map[string]string{"op": "read"})
	h.Observe(0.5)
	h.Observe(3)
	h.Observe(20)

	mf, _ := reg.Gather()
	if len(mf) != 1 || mf[0].GetName() != "latency" {
		t.Fatalf("unexpected family: %+v", mf)
	}
	hist := mf[0].Metric[0].Histogram
	if hist.GetSampleCount() != 3 {
		t.Fatalf("expected count 3, got %v", hist.GetSampleCount())
	}
	if hist.GetSampleSum() != 23.5 {
		t.Fatalf("expected sum 23.5, got %v", hist.GetSampleSum())
	}
}

func TestGauge_SetIncDec(t *testing.T) {
	m, reg := newIsolatedMeter()
	g := m.Gauge("queue_size", map[string]string{"q": "default"})
	g.Set(10)
	g.Inc()
	g.Dec()
	g.Dec()

	mf, _ := reg.Gather()
	got := mf[0].Metric[0].Gauge.GetValue()
	if got != 9 {
		t.Fatalf("expected 9, got %v", got)
	}
}

func TestHTTPHandler_ExposesMetrics(t *testing.T) {
	// DefaultRegisterer 模式，使用 HTTPHandler 默认走 DefaultGatherer
	reg := prometheus.NewRegistry()
	mm := New(WithRegisterer(reg))
	mm.Counter("exposed", nil).Inc()

	// HTTPHandler 是 promhttp.Handler()，默认走 DefaultGatherer
	// 验证它能 200 + 包含 prometheus 输出
	ts := httptest.NewServer(HTTPHandler())
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	if !strings.Contains(string(body[:n]), "# HELP") {
		t.Fatalf("expected prometheus output, got: %s", body[:n])
	}
}

func TestCacheKey_SortedByLabelKeys(t *testing.T) {
	k1 := cacheKey("foo", map[string]string{"z": "1", "a": "2", "m": "3"})
	k2 := cacheKey("foo", map[string]string{"a": "2", "m": "3", "z": "1"})
	if k1 != k2 {
		t.Fatalf("cache key 应与 label 顺序无关：%s vs %s", k1, k2)
	}
	if !strings.Contains(k1, "a,m,z") {
		t.Fatalf("expected sorted keys in cache key: %s", k1)
	}
}

func TestNew_Defaults(t *testing.T) {
	// 不应 panic；返回非 nil
	if got := New(); got == nil {
		t.Fatalf("expected non-nil Meter")
	}
}
