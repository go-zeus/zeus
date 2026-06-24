// Package prometheus 提供基于 prometheus/client_golang 的 Meter 实现。
//
// 设计要点：
//   - 同 (name, sortedLabelKeys) 组合共享一个 *CounterVec/HistogramVec/GaugeVec
//     避免重复 register 导致 panic
//   - 用户 labels map 顺序无关，内部对 keys 排序后做 cache key
//   - HTTPHandler() 返回 /metrics 暴露端点，用户挂到自己的 mux
//
// 用法：
//
//	meter := prometheus.New()
//	app := components.NewApp(
//	    components.NewMetricsComponent(meter),
//	    components.NewServerComponent(
//	        http.NewHTTP(http.Mux(myMux)),
//	    ),
//	)
//	// myMux.Handle("/metrics", prometheus.HTTPHandler())
package prometheus

import (
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/go-zeus/zeus/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Option 函数式选项
type Option func(*meter)

// WithRegisterer 自定义 Registerer（默认 prometheus.DefaultRegisterer）。
// 用于测试隔离或多 registry 场景。
func WithRegisterer(r prometheus.Registerer) Option {
	return func(m *meter) {
		if r != nil {
			m.reg = r
		}
	}
}

// WithNamespace 设置指标命名空间前缀（如 "zeus"），所有指标变为 `<ns>_<name>`。
func WithNamespace(ns string) Option {
	return func(m *meter) { m.namespace = ns }
}

// WithSubsystem 设置子系统前缀，所有指标变为 `<ns>_<sub>_<name>`。
func WithSubsystem(sub string) Option {
	return func(m *meter) { m.subsystem = sub }
}

// WithDefaultBuckets 设置 Histogram 默认 bucket 边界。
// 不调用时使用 prometheus.DefBuckets。
func WithDefaultBuckets(buckets []float64) Option {
	return func(m *meter) {
		if len(buckets) > 0 {
			m.buckets = append([]float64(nil), buckets...)
		}
	}
}

// 编译期检查 meter 实现 metrics.Meter
var _ metrics.Meter = (*meter)(nil)

type meter struct {
	reg       prometheus.Registerer
	namespace string
	subsystem string
	buckets   []float64

	counters   sync.Map // key: cacheKey(name, sortedLabelKeys) -> *prometheus.CounterVec
	histograms sync.Map
	gauges     sync.Map
}

// New 创建 Prometheus Meter。
//
// 默认值：
//   - Registerer: prometheus.DefaultRegisterer
//   - Namespace/Subsystem: 空
//   - Buckets: prometheus.DefBuckets
func New(opts ...Option) metrics.Meter {
	m := &meter{
		reg:     prometheus.DefaultRegisterer,
		buckets: prometheus.DefBuckets,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// HTTPHandler 返回 /metrics 暴露 HTTP handler。
// 用户在自己的 mux 上挂载：mux.Handle("/metrics", prometheus.HTTPHandler())
func HTTPHandler() http.Handler {
	return promhttp.Handler()
}

// cacheKey 由 metric name + sorted label keys 组成，保证 label 顺序无关
func cacheKey(name string, labels map[string]string) string {
	keys := sortedKeys(labels)
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('|')
	b.WriteString(strings.Join(keys, ","))
	return b.String()
}

func sortedKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func labelValues(keys []string, labels map[string]string) []string {
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = labels[k]
	}
	return vals
}

// registerCounterVec 注册 CounterVec，已存在时返回已有的。
// 避免并发或重复 Counter(name, labels) 调用导致 panic。
func (m *meter) registerCounterVec(cv *prometheus.CounterVec) *prometheus.CounterVec {
	if err := m.reg.Register(cv); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
	}
	return cv
}

func (m *meter) Counter(name string, labels map[string]string) metrics.Counter {
	keys := sortedKeys(labels)
	key := cacheKey(name, labels)
	if v, ok := m.counters.Load(key); ok {
		return &counter{cv: v.(*prometheus.CounterVec), keys: keys, labels: labels}
	}
	cv := m.registerCounterVec(prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: m.namespace,
		Subsystem: m.subsystem,
		Name:      name,
	}, keys))
	actual, _ := m.counters.LoadOrStore(key, cv)
	return &counter{cv: actual.(*prometheus.CounterVec), keys: keys, labels: labels}
}

// registerHistogramVec 注册 HistogramVec，已存在时返回已有的。
func (m *meter) registerHistogramVec(hv *prometheus.HistogramVec) *prometheus.HistogramVec {
	if err := m.reg.Register(hv); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing
			}
		}
	}
	return hv
}

func (m *meter) Histogram(name string, labels map[string]string) metrics.Histogram {
	keys := sortedKeys(labels)
	key := cacheKey(name, labels)
	if v, ok := m.histograms.Load(key); ok {
		return &histogram{hv: v.(*prometheus.HistogramVec), keys: keys, labels: labels}
	}
	hv := m.registerHistogramVec(prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: m.namespace,
		Subsystem: m.subsystem,
		Name:      name,
		Buckets:   m.buckets,
	}, keys))
	actual, _ := m.histograms.LoadOrStore(key, hv)
	return &histogram{hv: actual.(*prometheus.HistogramVec), keys: keys, labels: labels}
}

// registerGaugeVec 注册 GaugeVec，已存在时返回已有的。
func (m *meter) registerGaugeVec(gv *prometheus.GaugeVec) *prometheus.GaugeVec {
	if err := m.reg.Register(gv); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				return existing
			}
		}
	}
	return gv
}

func (m *meter) Gauge(name string, labels map[string]string) metrics.Gauge {
	keys := sortedKeys(labels)
	key := cacheKey(name, labels)
	if v, ok := m.gauges.Load(key); ok {
		return &gauge{gv: v.(*prometheus.GaugeVec), keys: keys, labels: labels}
	}
	gv := m.registerGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: m.namespace,
		Subsystem: m.subsystem,
		Name:      name,
	}, keys))
	actual, _ := m.gauges.LoadOrStore(key, gv)
	return &gauge{gv: actual.(*prometheus.GaugeVec), keys: keys, labels: labels}
}

func (m *meter) Close() error { return nil }

// 内部具体实现

type counter struct {
	cv     *prometheus.CounterVec
	keys   []string
	labels map[string]string
}

func (c *counter) Inc()          { c.cv.WithLabelValues(labelValues(c.keys, c.labels)...).Inc() }
func (c *counter) Add(v float64) { c.cv.WithLabelValues(labelValues(c.keys, c.labels)...).Add(v) }

type histogram struct {
	hv     *prometheus.HistogramVec
	keys   []string
	labels map[string]string
}

func (h *histogram) Observe(v float64) {
	h.hv.WithLabelValues(labelValues(h.keys, h.labels)...).Observe(v)
}

type gauge struct {
	gv     *prometheus.GaugeVec
	keys   []string
	labels map[string]string
}

func (g *gauge) Set(v float64) { g.gv.WithLabelValues(labelValues(g.keys, g.labels)...).Set(v) }
func (g *gauge) Inc()          { g.gv.WithLabelValues(labelValues(g.keys, g.labels)...).Inc() }
func (g *gauge) Dec()          { g.gv.WithLabelValues(labelValues(g.keys, g.labels)...).Dec() }
