// Package metrics 提供指标采集中间件。
//
// 行为：
//   - 每次请求计数一次（counter）
//   - labels 维度：cluster + method + status
//   - 可选：通过 WithBaggageLabels 显式声明哪些 baggage key 作为 label
//     （默认不启用，避免高基数 label 撑爆 Prometheus）
//   - meter 为 nil 时 no-op
//
// 用法：
//
//	import metricsmw "github.com/go-zeus/zeus/plugins/middleware/metrics"
//	import "github.com/go-zeus/zeus/metrics/noop"
//
//	// 默认：仅 cluster/method/status 维度
//	chain := middleware.NewChain(metricsmw.New(noop.New()))
//
//	// 高级：把 baggage 中的 tenant.id 作为 label（注意 cardinality 风险）
//	chain = middleware.NewChain(metricsmw.New(noop.New(),
//	    metricsmw.WithBaggageLabels("tenant.id"),
//	))
package metrics

import (
	"context"
	"strconv"

	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
)

// MetricRequestsTotal 请求计数指标名
const MetricRequestsTotal = "zeus_requests_total"

// LabelCluster 集群 label key
const LabelCluster = "cluster"

// LabelMethod 方法 label key
const LabelMethod = "method"

// LabelStatus 状态码 label key
const LabelStatus = "status"

// 编译期检查 metricsInterceptor 实现 middleware.Interceptor
var _ middleware.Interceptor = (*metricsInterceptor)(nil)

type metricsInterceptor struct {
	meter         metrics.Meter
	baggageLabels map[string]struct{} // 用户显式声明要作为 label 的 baggage key 白名单
}

// Option 配置 metrics 中间件行为
type Option func(*metricsInterceptor)

// WithBaggageLabels 声明哪些 baggage key 作为 metric label。
//
// 注意 cardinality 风险：如果 baggage key 的值空间很大（如 user.id），
// 会导致 Prometheus label 组合爆炸。仅声明值域有限的 key（如 tenant.id / region）。
//
// 未声明的 baggage key 不会被加成 label。
func WithBaggageLabels(keys ...string) Option {
	return func(m *metricsInterceptor) {
		if m.baggageLabels == nil {
			m.baggageLabels = make(map[string]struct{}, len(keys))
		}
		for _, k := range keys {
			if k != "" {
				m.baggageLabels[k] = struct{}{}
			}
		}
	}
}

// New 创建 metrics 中间件。
// meter 为 nil 时 no-op。
func New(meter metrics.Meter, opts ...Option) middleware.Interceptor {
	m := &metricsInterceptor{meter: meter}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

func (m *metricsInterceptor) Intercept(ctx context.Context, req middleware.Request, handler middleware.Handler) (middleware.Response, error) {
	resp, err := handler(ctx, req)

	// meter 未注入：跳过
	if m.meter == nil {
		return resp, err
	}

	// 收集 labels
	labels := map[string]string{
		LabelCluster: routing.FromContext(ctx),
	}
	if req != nil {
		labels[LabelMethod] = req.Method()
	}
	if resp != nil {
		// 把 status code 转为字符串
		s := resp.StatusCode()
		if s == 0 {
			s = 200
		}
		labels[LabelStatus] = strconv.Itoa(s)
	} else if err != nil {
		labels[LabelStatus] = "500"
	} else {
		labels[LabelStatus] = "200"
	}

	// 按用户白名单追加 baggage labels
	if len(m.baggageLabels) > 0 {
		if bag := propagation.FromContext(ctx); bag != nil {
			for key := range m.baggageLabels {
				if v, ok := bag.Get(key); ok {
					labels[key] = v
				}
			}
		}
	}

	m.meter.Counter(MetricRequestsTotal, labels).Inc()
	return resp, err
}

func (m *metricsInterceptor) Name() string {
	return "metrics"
}
