package metrics

// Counter 计数器接口
type Counter interface {
	Inc()
	Add(float64)
}

// Histogram 直方图接口
type Histogram interface {
	Observe(float64)
}

// Gauge 仪表盘接口
type Gauge interface {
	Set(float64)
	Inc()
	Dec()
}

// Meter 指标管理器接口
type Meter interface {
	// Counter 创建或获取计数器
	Counter(name string, labels map[string]string) Counter
	// Histogram 创建或获取直方图
	Histogram(name string, labels map[string]string) Histogram
	// Gauge 创建或获取仪表盘
	Gauge(name string, labels map[string]string) Gauge
	// Close 关闭
	Close() error
}
