// Package clustering 提供集群路由（X-Zeus-Cluster）透传中间件。
//
// 从入站请求提取集群标记（cluster）写入 context，确保下游调用自动传播：
//   - HTTP：从 X-Zeus-Cluster Header 读取
//   - context 已有 cluster 标记时优先使用
//
// 与 server/http.clusterInjector 的差异：
//   - clustering.New() 适配 middleware.Interceptor 接口，用于 middleware.Chain
//   - clusterInjector 是 http.Handler 包装器，用于 server/http 直接接入
package clustering

import (
	"context"
	"net/http"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/routing"
)

// 编译期检查 clusteringInterceptor 实现了 middleware.Interceptor 接口
var _ middleware.Interceptor = (*clusteringInterceptor)(nil)

type clusteringInterceptor struct{}

// New 创建集群路由透传中间件
// 从入站请求提取集群标记写入 context，确保下游调用自动传播
func New() middleware.Interceptor {
	return &clusteringInterceptor{}
}

func (c *clusteringInterceptor) Intercept(ctx context.Context, req middleware.Request, handler middleware.Handler) (middleware.Response, error) {
	cluster := extractCluster(ctx, req)
	ctx = routing.WithCluster(ctx, cluster)
	return handler(ctx, req)
}

func (c *clusteringInterceptor) Name() string {
	return "clustering"
}

// extractCluster 从 context 和 request 中提取集群标记
// 优先级：context 已有 > request header
func extractCluster(ctx context.Context, req middleware.Request) string {
	// 如果 context 已有非默认 cluster 标记，直接使用
	if cluster := routing.FromContext(ctx); !routing.IsDefault(cluster) {
		return cluster
	}
	// 从 request header 提取
	if req != nil {
		if cluster := req.Header(routing.HeaderCluster); cluster != "" {
			return cluster
		}
	}
	return routing.Default
}

// ClusterFromHTTP 从 HTTP 请求提取集群标记并注入 context。
//
// 典型场景：server 端 HTTP handler 入口处调用，将 X-Zeus-Cluster Header
// 转为 context 内的 cluster 字段，供下游中间件 / 业务代码读取。
func ClusterFromHTTP(r *http.Request) context.Context {
	cluster := r.Header.Get(routing.HeaderCluster)
	if cluster == "" {
		cluster = routing.Default
	}
	return routing.WithCluster(r.Context(), cluster)
}
