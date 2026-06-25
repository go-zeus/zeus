package routing

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-zeus/zeus/propagation"
)

// TestWithCluster_FromContext_RoundTrip 验证 context 注入与读取的往返一致性
func TestWithCluster_FromContext_RoundTrip(t *testing.T) {
	ctx := WithCluster(context.Background(), "canary")
	if got := FromContext(ctx); got != "canary" {
		t.Fatalf("FromContext = %q, want canary", got)
	}
}

// TestFromContext_Empty 验证空 context 返回 Default
func TestFromContext_Empty(t *testing.T) {
	if got := FromContext(context.Background()); got != Default {
		t.Fatalf("FromContext(empty) = %q, want %q", got, Default)
	}
}

// TestWithCluster_EmptyString 验证空字符串会被规范化为 Default
func TestWithCluster_EmptyString(t *testing.T) {
	ctx := WithCluster(context.Background(), "")
	if got := FromContext(ctx); got != Default {
		t.Fatalf("FromContext(empty cluster) = %q, want %q", got, Default)
	}
}

// TestClusterFromHTTPHeader 验证 HTTP Header 提取
func TestClusterFromHTTPHeader(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderCluster, "canary")
	if got := ClusterFromHTTPHeader(h); got != "canary" {
		t.Fatalf("ClusterFromHTTPHeader = %q, want canary", got)
	}

	h2 := http.Header{}
	if got := ClusterFromHTTPHeader(h2); got != Default {
		t.Fatalf("ClusterFromHTTPHeader(empty) = %q, want %q", got, Default)
	}
}

// TestClusterFromMetadata 验证 gRPC metadata 风格的 map 提取
func TestClusterFromMetadata(t *testing.T) {
	md := map[string]string{MetadataCluster: "order.v2"}
	if got := ClusterFromMetadata(md); got != "order.v2" {
		t.Fatalf("ClusterFromMetadata = %q, want order.v2", got)
	}

	if got := ClusterFromMetadata(nil); got != Default {
		t.Fatalf("ClusterFromMetadata(nil) = %q, want %q", got, Default)
	}
}

// TestIsDefault 验证 Default 判断
func TestIsDefault(t *testing.T) {
	if !IsDefault("") {
		t.Error("IsDefault(\"\") should be true")
	}
	if !IsDefault(Default) {
		t.Error("IsDefault(Default) should be true")
	}
	if IsDefault("canary") {
		t.Error("IsDefault(\"canary\") should be false")
	}
}

// TestConstants 验证常量值符合预期（与外部协议契约一致）
func TestConstants(t *testing.T) {
	if HeaderCluster != "X-Zeus-Cluster" {
		t.Fatalf("HeaderCluster = %q, want X-Zeus-Cluster", HeaderCluster)
	}
	if MetadataCluster != "x-zeus-cluster" {
		t.Fatalf("MetadataCluster = %q, want x-zeus-cluster", MetadataCluster)
	}
	if Default != "default" {
		t.Fatalf("Default = %q, want default", Default)
	}
	if BagKey != "zeus.cluster" {
		t.Fatalf("BagKey = %q, want zeus.cluster", BagKey)
	}
}

// TestWithCluster_SyncsToPropagation WithCluster 同时写入 propagation Bag
func TestWithCluster_SyncsToPropagation(t *testing.T) {
	ctx := WithCluster(context.Background(), "canary")
	v, ok := propagation.Get(ctx, BagKey)
	if !ok {
		t.Fatal("propagation.Get should return ok=true after WithCluster")
	}
	if v != "canary" {
		t.Errorf("propagation.Get(zeus.cluster) = %q, want canary", v)
	}
}

// TestFromContext_ReadsFromPropagation 当 ctxKey 缺失时，从 propagation Bag 兜底
func TestFromContext_ReadsFromPropagation(t *testing.T) {
	// 仅写入 propagation（模拟从入站 baggage extract 后的 ctx）
	ctx := propagation.With(context.Background(), BagKey, "canary")
	if got := FromContext(ctx); got != "canary" {
		t.Fatalf("FromContext = %q, want canary (from propagation)", got)
	}
}

// TestFromContext_CtxKeyBeatsPropagation ctxKey 优先级高于 propagation
//
// 设计动机：本地 WithCluster 是最新写入，应覆盖 propagation 中的旧值。
func TestFromContext_CtxKeyBeatsPropagation(t *testing.T) {
	ctx := propagation.With(context.Background(), BagKey, "from-baggage")
	ctx = WithCluster(ctx, "from-local")
	if got := FromContext(ctx); got != "from-local" {
		t.Fatalf("FromContext = %q, want from-local (ctxKey wins)", got)
	}
}

// TestRoundTrip_HTTPBaggagePropagation 端到端：baggage header 透传 cluster
func TestRoundTrip_HTTPBaggagePropagation(t *testing.T) {
	// 上游：WithCluster → client 自动注入 baggage header
	ctx := WithCluster(context.Background(), "canary")
	hdr := http.Header{}
	propagation.InjectHTTP(ctx, hdr)

	// 下游：从 baggage header extract
	extracted := propagation.ExtractHTTP(context.Background(), hdr)
	// routing.FromContext 应能读到（通过 propagation Bag 兜底）
	if got := FromContext(extracted); got != "canary" {
		t.Errorf("FromContext(extracted) = %q, want canary", got)
	}
}
