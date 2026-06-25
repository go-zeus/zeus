// Package tracing 提供链路追踪中间件。
//
// 行为：
//   - 在请求入口创建 span
//   - 自动从 context 提取 cluster，写入 span attribute "zeus.cluster"
//   - 自动从 context 提取 baggage entries，全部写入 span attribute（key 用原值）
//   - 把 span 注入 context，供下游 tracer 使用
//
// 用法：
//
//	import tracing "github.com/go-zeus/zeus/plugins/middleware/tracing"
//	import "github.com/go-zeus/zeus/trace/noop"
//
//	chain := middleware.NewChain(tracing.New(noop.New()))
package tracing

import (
	"context"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/trace"
)

// AttributeCluster span attribute 中集群标记的 key
const AttributeCluster = "zeus.cluster"

// 编译期检查 tracingInterceptor 实现了 middleware.Interceptor 接口
var _ middleware.Interceptor = (*tracingInterceptor)(nil)

type tracingInterceptor struct {
	tracer trace.Tracer
}

// New 创建 tracing 中间件。
// tracer 为 nil 时返回的拦截器为 no-op（不创建 span）。
func New(tracer trace.Tracer) middleware.Interceptor {
	return &tracingInterceptor{tracer: tracer}
}

func (t *tracingInterceptor) Intercept(ctx context.Context, req middleware.Request, handler middleware.Handler) (middleware.Response, error) {
	// tracer 未注入：直接透传
	if t.tracer == nil {
		return handler(ctx, req)
	}

	name := "request"
	if req != nil && req.Method() != "" {
		// 拼 method + path 作为 span 名
		name = req.Method() + " " + req.Path()
	}

	spanCtx, span := t.tracer.StartSpan(ctx, name)
	defer span.End()

	// 自动注入集群 attribute（显式独立 key，便于查询过滤）
	if c := routing.FromContext(ctx); !routing.IsDefault(c) {
		span.SetAttributes(map[string]string{AttributeCluster: c})
	}

	// 自动注入 baggage entries 作为 span attribute（用户自定义 K-V）
	// zeus.cluster 已由上面单独处理，这里跳过避免重复
	if bag := propagation.FromContext(ctx); bag != nil && bag.Len() > 0 {
		attrs := make(map[string]string, bag.Len())
		for _, e := range bag.Entries() {
			if e.Key == routing.BagKey {
				continue
			}
			attrs[e.Key] = e.Value
		}
		if len(attrs) > 0 {
			span.SetAttributes(attrs)
		}
	}

	return handler(spanCtx, req)
}

func (t *tracingInterceptor) Name() string {
	return "tracing"
}
