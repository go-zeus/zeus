package propagation

import (
	"context"
	"net/http"
	"testing"
)

// propagation 是全链路上下文传播的核心：每个 HTTP/gRPC 请求出入都会触发
// Inject/Extract，每个业务读值都走 Get。本 bench 覆盖：
//   - 业务高频：With / Get / FromContext
//   - 跨进程热路径：Encode / Decode / InjectHTTP / ExtractHTTP
//   - 不同 Bag 规模下的编解码扩展性（1 vs 10 entries）
//
// Bag 是不可变结构（每次 With 返回新 Bag），alloc 数据反映 context 派生开销。

// makeCtx 构造带 n 个 entry 的 ctx
func makeCtx(n int) context.Context {
	ctx := context.Background()
	for i := 0; i < n; i++ {
		ctx = With(ctx, key(i), val(i))
	}
	return ctx
}

func key(i int) string { return []string{"k0", "k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9"}[i%10] }
func val(i int) string { return []string{"v0", "v1", "v2", "v3", "v4", "v5", "v6", "v7", "v8", "v9"}[i%10] }

// --- 业务高频 ---

// BenchmarkWith 单次派生 ctx（典型：业务代码 propagation.With）
func BenchmarkWith(b *testing.B) {
	ctx := makeCtx(5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = With(ctx, "new-key", "new-val")
	}
}

// BenchmarkGet 读取单个 key（FromContext + Bag.Get）
func BenchmarkGet(b *testing.B) {
	ctx := makeCtx(5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Get(ctx, "k0")
	}
}

// BenchmarkFromContext 取 Bag 视图（单次 ctx.Value 断言）
func BenchmarkFromContext(b *testing.B) {
	ctx := makeCtx(5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FromContext(ctx)
	}
}

// --- 跨进程编解码 ---

// BenchmarkEncode_1 编码 1 个 entry（最小开销基线）
func BenchmarkEncode_1(b *testing.B) {
	bag := FromContext(makeCtx(1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Encode(bag)
	}
}

// BenchmarkEncode_10 编码 10 个 entry（量化每 entry 边际成本）
func BenchmarkEncode_10(b *testing.B) {
	bag := FromContext(makeCtx(10))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Encode(bag)
	}
}

// BenchmarkDecode_1 解码 1 个 entry
func BenchmarkDecode_1(b *testing.B) {
	raw := Encode(FromContext(makeCtx(1)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Decode(raw)
	}
}

// BenchmarkDecode_10 解码 10 个 entry
func BenchmarkDecode_10(b *testing.B) {
	raw := Encode(FromContext(makeCtx(10)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Decode(raw)
	}
}

// --- HTTP 出入站 ---

// BenchmarkInjectHTTP 注入 5 个 entry 到 header（出口热路径）
func BenchmarkInjectHTTP(b *testing.B) {
	ctx := makeCtx(5)
	hdr := http.Header{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hdr.Del(HeaderBaggage) // 模拟干净 header
		InjectHTTP(ctx, hdr)
	}
}

// BenchmarkExtractHTTP 从 header 提取（入口热路径）
func BenchmarkExtractHTTP(b *testing.B) {
	srcHdr := http.Header{}
	InjectHTTP(makeCtx(5), srcHdr)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractHTTP(ctx, srcHdr)
	}
}

// BenchmarkExtractHTTP_Empty 无 Baggage header 的快速路径（绝大多数请求）
func BenchmarkExtractHTTP_Empty(b *testing.B) {
	hdr := http.Header{} // 无 Baggage
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractHTTP(ctx, hdr)
	}
}

// --- gRPC metadata 出入站 ---

// BenchmarkInjectMetadataMulti gRPC 出口（多值 metadata）
func BenchmarkInjectMetadataMulti(b *testing.B) {
	ctx := makeCtx(5)
	md := map[string][]string{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		delete(md, HeaderBaggage)
		InjectMetadataMulti(ctx, md)
	}
}

// BenchmarkExtractMetadataMulti gRPC 入口
func BenchmarkExtractMetadataMulti(b *testing.B) {
	srcMD := map[string][]string{}
	InjectMetadataMulti(makeCtx(5), srcMD)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractMetadataMulti(ctx, srcMD)
	}
}
