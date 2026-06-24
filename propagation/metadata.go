// gRPC metadata 风格的注入与提取。
//
// 与 HTTP Header 对齐，但使用 gRPC metadata key "baggage"（小写，符合 gRPC 规范）。
// gRPC metadata 在 Go 中的体现通常是 map[string]string 或 map[string][]string。
//
// 设计为 map[string]string 操作，避免直接依赖 google.golang.org/grpc/metadata
// （保持主包零依赖；plugins/client/grpc 和 plugins/server/grpc 调用时做适配）。
package propagation

import (
	"context"
	"strings"
)

// InjectMetadata 将 ctx 中的 Bag 写入 map[string]string。
//
// 行为：
//   - ctx 无 Bag → no-op（保留 md 已有 baggage）
//   - ctx 有 Bag → 覆盖 md["baggage"]（覆盖语义）
//   - md nil → no-op
//
// 适合 gRPC outgoing metadata 构造场景。
func InjectMetadata(ctx context.Context, md map[string]string) {
	if ctx == nil || md == nil {
		return
	}
	bag := FromContext(ctx)
	if bag == nil || bag.Len() == 0 {
		return
	}
	md[MetadataBaggage] = Encode(bag)
}

// ExtractMetadata 从 map[string]string 解析 baggage 并合并到 ctx 中。
//
// 行为与 ExtractHTTP 对齐：
//   - md 无 baggage key → 返回原 ctx
//   - md 有 baggage → 解析后合并到 ctx（同 Key 覆盖）
func ExtractMetadata(ctx context.Context, md map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if md == nil {
		return ctx
	}
	raw, ok := md[MetadataBaggage]
	if !ok || strings.TrimSpace(raw) == "" {
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

// InjectMetadataMulti 写入 map[string][]string（gRPC metadata 的常见形式）。
//
// 覆盖语义：md["baggage"] = []string{encoded}
func InjectMetadataMulti(ctx context.Context, md map[string][]string) {
	if ctx == nil || md == nil {
		return
	}
	bag := FromContext(ctx)
	if bag == nil || bag.Len() == 0 {
		return
	}
	md[MetadataBaggage] = []string{Encode(bag)}
}

// ExtractMetadataMulti 从 map[string][]string 解析 baggage。
//
// 行为：取 md["baggage"] 第一个非空值（多值时按 W3C 规范用 "," 拼接后统一解码）。
func ExtractMetadataMulti(ctx context.Context, md map[string][]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if md == nil {
		return ctx
	}
	values, ok := md[MetadataBaggage]
	if !ok || len(values) == 0 {
		return ctx
	}
	// gRPC metadata 允许同名 key 多值，按 W3C baggage 协议用 "," 合并
	raw := strings.Join(values, ",")
	if strings.TrimSpace(raw) == "" {
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
