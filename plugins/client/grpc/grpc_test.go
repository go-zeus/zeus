package grpc

import (
	"context"
	"testing"

	"github.com/go-zeus/zeus/routing"
	"google.golang.org/grpc"
	grpcmeta "google.golang.org/grpc/metadata"
)

// TestUnaryInterceptor_InjectsCluster 验证 ctx 含 cluster 时注入到 outgoing metadata
func TestUnaryInterceptor_InjectsCluster(t *testing.T) {
	interceptor := UnaryInterceptor()

	ctx := routing.WithCluster(context.Background(), "canary")
	var capturedCtx context.Context

	invoker := func(c context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = c
		return nil
	}

	_ = interceptor(ctx, "/svc/Method", nil, nil, nil, invoker)

	md, ok := grpcmeta.FromOutgoingContext(capturedCtx)
	if !ok {
		t.Fatal("outgoing metadata not set")
	}
	vals := md.Get(routing.MetadataCluster)
	if len(vals) != 1 || vals[0] != "canary" {
		t.Errorf("metadata[%s] = %v, want [canary]", routing.MetadataCluster, vals)
	}
}

// TestUnaryInterceptor_DefaultCluster_NotInjected 验证 default cluster 不污染 metadata
func TestUnaryInterceptor_DefaultCluster_NotInjected(t *testing.T) {
	interceptor := UnaryInterceptor()

	var capturedCtx context.Context
	invoker := func(c context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = c
		return nil
	}

	_ = interceptor(context.Background(), "/svc/Method", nil, nil, nil, invoker)

	if _, ok := grpcmeta.FromOutgoingContext(capturedCtx); ok {
		t.Error("default cluster should not create outgoing metadata")
	}
}

// TestUnaryInterceptor_PreservesExistingMetadata 验证已有 metadata 不被覆盖
func TestUnaryInterceptor_PreservesExistingMetadata(t *testing.T) {
	interceptor := UnaryInterceptor()

	// 预设一个 trace-id
	ctx := grpcmeta.NewOutgoingContext(context.Background(),
		grpcmeta.Pairs("x-trace-id", "abc123"))
	ctx = routing.WithCluster(ctx, "canary")

	var capturedCtx context.Context
	invoker := func(c context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = c
		return nil
	}

	_ = interceptor(ctx, "/svc/Method", nil, nil, nil, invoker)

	md, _ := grpcmeta.FromOutgoingContext(capturedCtx)
	if v := md.Get("x-trace-id"); len(v) != 1 || v[0] != "abc123" {
		t.Errorf("existing metadata lost: %v", v)
	}
	if v := md.Get(routing.MetadataCluster); len(v) != 1 || v[0] != "canary" {
		t.Errorf("cluster not injected: %v", v)
	}
}

// TestWithCluster 验证手动注入 cluster
func TestWithCluster(t *testing.T) {
	ctx := WithCluster(context.Background(), "canary")
	md, ok := grpcmeta.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("metadata not set")
	}
	if v := md.Get(routing.MetadataCluster); len(v) != 1 || v[0] != "canary" {
		t.Errorf("got %v, want [canary]", v)
	}
}

// TestWithCluster_DefaultCluster_NoOp 验证 default cluster 不修改 ctx
func TestWithCluster_DefaultCluster_NoOp(t *testing.T) {
	ctx := WithCluster(context.Background(), routing.Default)
	if _, ok := grpcmeta.FromOutgoingContext(ctx); ok {
		t.Error("default cluster should not create metadata")
	}
}

// TestWithCluster_OverwritesPreviousValue 验证多次设置 cluster 时只保留最新值
// 防止 Append 模式导致 vals[0] 取到陈旧 cluster
func TestWithCluster_OverwritesPreviousValue(t *testing.T) {
	ctx := WithCluster(context.Background(), "canary")
	ctx = WithCluster(ctx, "prod")
	md, ok := grpcmeta.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("metadata not set")
	}
	vals := md.Get(routing.MetadataCluster)
	if len(vals) != 1 || vals[0] != "prod" {
		t.Errorf("expected single [prod], got %v", vals)
	}
}

// TestUnaryInterceptor_DoesNotAccumulateCluster 验证拦截器重复触发不累积 cluster 条目
func TestUnaryInterceptor_DoesNotAccumulateCluster(t *testing.T) {
	interceptor := UnaryInterceptor()

	ctx := routing.WithCluster(context.Background(), "canary")
	var capturedCtx context.Context
	invoker := func(c context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = c
		return nil
	}

	// 同一 ctx 连续经过两次拦截器（模拟嵌套调用）
	_ = interceptor(ctx, "/svc/M1", nil, nil, nil, invoker)
	_ = interceptor(capturedCtx, "/svc/M2", nil, nil, nil, invoker)

	md, _ := grpcmeta.FromOutgoingContext(capturedCtx)
	vals := md.Get(routing.MetadataCluster)
	if len(vals) != 1 || vals[0] != "canary" {
		t.Errorf("expected single [canary], got %v", vals)
	}
}
