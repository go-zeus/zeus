// Package grpc 提供 gRPC 客户端包装器，自动从 context 提取集群标记和 baggage
// 注入到 outgoing metadata。
//
// 行为：
//   - UnaryClientInterceptor：从 ctx 提取 cluster → metadata["x-zeus-cluster"]
//   - UnaryClientInterceptor：从 ctx 提取 baggage → metadata["baggage"]（W3C 标准格式）
//   - 默认 cluster 不写入 metadata（避免污染下游日志）
//   - baggage 仅在 ctx 中存在 K-V 时写入
//
// v1 仅提供拦截器（最小依赖），用户可绑定到任意 grpc.ClientConn。
// v2 将提供完整 Client（含服务发现 + 负载均衡）。
//
// 用法：
//
//	import grpcclient "github.com/go-zeus/zeus/plugins/client/grpc"
//
//	cc, _ := grpc.NewClient(target,
//	    grpc.WithUnaryInterceptor(grpcclient.UnaryInterceptor()),
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	)
package grpc

import (
	"context"

	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
	"google.golang.org/grpc"
	grpcmeta "google.golang.org/grpc/metadata"
)

// UnaryInterceptor 创建 UnaryClientInterceptor，自动注入 cluster 和 baggage 到 metadata。
//
// 行为：
//   - ctx 中已含非默认 cluster：Set 到 outgoing metadata（覆盖任何旧值）
//   - 默认 cluster：不修改 metadata（避免噪音）
//   - ctx 中有 propagation Bag：把 baggage 编码后写入 metadata["baggage"]
//   - 用户已有 metadata：保留其余 key，只更新 cluster / baggage
//
// 注意：使用 Set（非 Append）避免多次调用产生重复 cluster 条目，
// 否则下游 clusterInterceptor 取 vals[0] 会拿到陈旧值。
func UnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		c := routing.FromContext(ctx)
		if !routing.IsDefault(c) {
			ctx = setOutgoingCluster(ctx, c)
		}
		// 自动注入 baggage（用户自定义 K-V）
		ctx = injectBaggage(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// WithCluster 显式设置 outgoing metadata 的 cluster，返回新 ctx。
//
// 用于在不通过 interceptor 的场景（如直接调用 grpc.Invoke）下手动注入。
func WithCluster(ctx context.Context, c string) context.Context {
	if routing.IsDefault(c) {
		return ctx
	}
	return setOutgoingCluster(ctx, c)
}

// setOutgoingCluster 把 cluster 写入 outgoing metadata，覆盖同名旧值。
func setOutgoingCluster(ctx context.Context, c string) context.Context {
	md, ok := grpcmeta.FromOutgoingContext(ctx)
	if !ok {
		md = grpcmeta.Pairs(routing.MetadataCluster, c)
	} else {
		md = md.Copy()
		md.Set(routing.MetadataCluster, c)
	}
	return grpcmeta.NewOutgoingContext(ctx, md)
}

// injectBaggage 把 ctx 中的 propagation Bag 写入 outgoing metadata。
//
// 行为：
//   - ctx 无 Bag → 返回原 ctx（不污染 metadata）
//   - ctx 有 Bag → 设置 md["baggage"] = [encoded]（覆盖语义）
func injectBaggage(ctx context.Context) context.Context {
	bag := propagation.FromContext(ctx)
	if bag == nil || bag.Len() == 0 {
		return ctx
	}
	encoded := propagation.Encode(bag)
	md, ok := grpcmeta.FromOutgoingContext(ctx)
	if !ok {
		md = grpcmeta.Pairs(propagation.MetadataBaggage, encoded)
	} else {
		md = md.Copy()
		md.Set(propagation.MetadataBaggage, encoded)
	}
	return grpcmeta.NewOutgoingContext(ctx, md)
}
