// HTTP Header 注入与提取。
//
// 与 W3C Baggage 规范对齐：
//   - HTTP Header 名：Baggage（大小写不敏感，http.Header.Set 自动做 Title Case）
//   - 多个 Entry 用 "," 分隔
//
// 使用：
//
//	// 出口（在 client 调用前）
//	propagation.InjectHTTP(ctx, req.Header)
//
//	// 入口（在 handler 第一行）
//	ctx = propagation.ExtractHTTP(r.Context(), r.Header)
package propagation

import (
	"context"
	"net/http"
)

// InjectHTTP 将 ctx 中的 Bag 注入到 HTTP Header。
//
// 行为：
//   - ctx 无 Bag → no-op（保留 hdr 中已有的 Baggage header）
//   - ctx 有 Bag → 覆盖 hdr["Baggage"]（覆盖语义保证一致性）
//   - hdr nil → no-op（防御性）
func InjectHTTP(ctx context.Context, hdr http.Header) {
	if ctx == nil || hdr == nil {
		return
	}
	bag := FromContext(ctx)
	if bag == nil || bag.Len() == 0 {
		return
	}
	hdr.Set(HeaderBaggage, Encode(bag))
}

// ExtractHTTP 从 HTTP Header 解析 Baggage 并合并到 ctx 中。
//
// 行为：
//   - hdr 无 Baggage header → 返回原 ctx
//   - hdr 有 Baggage header → Decode 后追加到 ctx 已有 Bag（同 Key 覆盖）
//
// 注意：合并语义而非"完全替换"，允许中间件在 extract 前预注入 K-V。
// 例：clusterInjector 先做 propagation.With(ctx, "zeus.cluster", "canary")，
// 然后 ExtractHTTP 解析出 "tenant=acme"，最终 ctx 同时有 cluster 和 tenant。
func ExtractHTTP(ctx context.Context, hdr http.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if hdr == nil {
		return ctx
	}
	raw := hdr.Get(HeaderBaggage)
	if raw == "" {
		return ctx
	}
	extracted := Decode(raw)
	if extracted == nil || extracted.Len() == 0 {
		return ctx
	}
	existing := FromContext(ctx)
	merged := existing.WithEntries(extracted.Entries()...)
	return context.WithValue(ctx, ctxKey{}, merged)
}

// HTTPMiddleware 包装 http.Handler，自动从入站 Header extract baggage 到 ctx。
//
// 与 server/http 的 clusterInjector 配合：clusterInjector 调 ExtractHTTP 后，
// 后续 handler 在 ctx 中既能拿到 zeus.cluster 也能拿到自定义 K-V。
//
// 使用场景：
//   - 用户不使用 zeus server，但希望 propagation 在自己的 HTTP 服务中生效
//   - 测试代码中快速包装 handler
//
// 不需要时（默认 server/http 已内置 extract）：不要重复应用，避免重复解码。
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ExtractHTTP(r.Context(), r.Header)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
