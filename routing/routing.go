// Package routing 提供集群路由（X-Zeus-Cluster）的统一 API。
//
// 设计目的：
//   - 解除 log/server 等主包与 client 包的潜在循环依赖
//   - 统一 HTTP / gRPC 协议下的集群标记提取与注入语义
//   - 对齐 K8s/Istio/Envoy/gRPC xDS 的 cluster 概念
//
// 规则：
//   - HTTP：通过 X-Zeus-Cluster Header 传递
//   - gRPC：通过 metadata["x-zeus-cluster"] 传递
//   - 任意一层缺失则回退到 Default
//
// 与 propagation 包的关系：
//   - routing 是 propagation 的特化，绑定常量 Key = "zeus.cluster"
//   - WithCluster 同时写入 ctx 本地值 + propagation Bag（同步传播）
//   - FromContext 优先读 ctx 本地值，缺失回退到 propagation Bag
//   - 这样既能享受 propagation 自动 baggage 透传，又保持 routing API 零开销
//
// client/middleware/proxy 等包通过本包实现，保持行为一致。
package routing

import (
	"context"
	"net/http"

	"github.com/go-zeus/zeus/propagation"
)

// ctxKey context 中集群标记的 key 类型（不导出，避免外部修改）
//
// 保留独立 ctxKey 的原因：cluster 是高频访问字段，
// routing.FromContext 不应每次都走 propagation Bag 的 map 查找。
// 同时仍同步到 propagation，保证下游服务能从 baggage 读取。
type ctxKey struct{}

// HeaderCluster 集群路由 HTTP Header 名称
const HeaderCluster = "X-Zeus-Cluster"

// MetadataCluster 集群路由 gRPC metadata key 名称（小写，符合 gRPC 规范）
const MetadataCluster = "x-zeus-cluster"

// Default 默认集群名称
const Default = "default"

// BagKey cluster 在 propagation Bag 中的 key 名（与 baggage header 同步传播）
//
// 设计动机：让 cluster 既走 X-Zeus-Cluster 显式 Header（易调试），
// 也走 Baggage 隐式 Header（用户自定义 K-V 时一并透传）。
// 两侧保持一致：WithCluster 写入 ctxKey + Bag；FromContext 优先读 ctxKey。
const BagKey = "zeus.cluster"

// WithCluster 向 context 注入集群标记。
//
// 副作用：同时写入 propagation Bag（key=BagKey），便于跨进程 baggage 自动透传。
// 传入空字符串等价于设置为 Default。
func WithCluster(ctx context.Context, c string) context.Context {
	if c == "" {
		c = Default
	}
	ctx = context.WithValue(ctx, ctxKey{}, c)
	// 同步到 propagation Bag（用户后续 client.Do 时自动 InjectHTTP）
	ctx = propagation.With(ctx, BagKey, c)
	return ctx
}

// FromContext 从 context 读取集群标记。
//
// 读取顺序：
//  1. ctxKey（本地 WithCluster 写入，O(1)）
//  2. propagation Bag（从入站 baggage extract 得到，跨进程场景）
//  3. Default
func FromContext(ctx context.Context) string {
	if c, ok := ctx.Value(ctxKey{}).(string); ok && c != "" {
		return c
	}
	if v, ok := propagation.Get(ctx, BagKey); ok && v != "" {
		return v
	}
	return Default
}

// ClusterFromHTTPHeader 从 HTTP Header 读取集群标记。
// 缺失返回 Default。
func ClusterFromHTTPHeader(h http.Header) string {
	if c := h.Get(HeaderCluster); c != "" {
		return c
	}
	return Default
}

// ClusterFromMetadata 从 KV 元数据（gRPC metadata 兼容）读取集群标记。
// 缺失返回 Default。
func ClusterFromMetadata(md map[string]string) string {
	if c, ok := md[MetadataCluster]; ok && c != "" {
		return c
	}
	return Default
}

// IsDefault 是否为默认集群
func IsDefault(c string) bool {
	return c == "" || c == Default
}
