package propagation

import (
	"context"
	"net/http"
	"testing"
)

// TestBag_WithAndGet 验证 Bag 的 K-V 往返
func TestBag_WithAndGet(t *testing.T) {
	bag := NewBag().With("a", "1").With("b", "2")
	if v, ok := bag.Get("a"); !ok || v != "1" {
		t.Errorf("Get(a) = (%q,%v), want (\"1\", true)", v, ok)
	}
	if v, ok := bag.Get("b"); !ok || v != "2" {
		t.Errorf("Get(b) = (%q,%v), want (\"2\", true)", v, ok)
	}
	if _, ok := bag.Get("c"); ok {
		t.Error("Get(c) should not exist")
	}
}

// TestBag_Overwrite 同 Key 后写覆盖
func TestBag_Overwrite(t *testing.T) {
	bag := NewBag().With("k", "v1").With("k", "v2")
	if v, _ := bag.Get("k"); v != "v2" {
		t.Errorf("after overwrite, Get(k) = %q, want v2", v)
	}
	if bag.Len() != 1 {
		t.Errorf("Len = %d, want 1 (no duplicate key)", bag.Len())
	}
}

// TestBag_EntriesPreservesOrder 顺序保留
func TestBag_EntriesPreservesOrder(t *testing.T) {
	bag := NewBag().
		With("zeus.cluster", "canary").
		With("tenant.id", "acme").
		With("feature.flag", "beta")
	entries := bag.Entries()
	want := []struct{ k, v string }{
		{"zeus.cluster", "canary"},
		{"tenant.id", "acme"},
		{"feature.flag", "beta"},
	}
	if len(entries) != len(want) {
		t.Fatalf("Entries len = %d, want %d", len(entries), len(want))
	}
	for i, e := range entries {
		if e.Key != want[i].k || e.Value != want[i].v {
			t.Errorf("Entries[%d] = %v, want %+v", i, e, want[i])
		}
	}
}

// TestBag_WithEntries 批量追加
func TestBag_WithEntries(t *testing.T) {
	bag := NewBag().WithEntries(
		Entry{Key: "a", Value: "1"},
		Entry{Key: "b", Value: "2"},
	)
	if bag.Len() != 2 {
		t.Fatalf("Len = %d, want 2", bag.Len())
	}
}

// TestBag_Immutability 修改 Bag 不影响原 Bag
func TestBag_Immutability(t *testing.T) {
	bag1 := NewBag().With("a", "1")
	bag2 := bag1.With("b", "2")
	if bag1.Len() != 1 {
		t.Errorf("bag1.Len = %d, want 1 (immutable)", bag1.Len())
	}
	if bag2.Len() != 2 {
		t.Errorf("bag2.Len = %d, want 2", bag2.Len())
	}
}

// TestBag_EmptyKey 空键忽略
func TestBag_EmptyKey(t *testing.T) {
	bag := NewBag().With("", "ignored")
	if bag.Len() != 0 {
		t.Errorf("Len = %d, want 0 (empty key ignored)", bag.Len())
	}
}

// TestBag_NilSafe nil Bag 操作安全
func TestBag_NilSafe(t *testing.T) {
	var bag *Bag
	if bag.Len() != 0 {
		t.Errorf("nil.Len() = %d, want 0", bag.Len())
	}
	if _, ok := bag.Get("any"); ok {
		t.Error("nil.Get should return false")
	}
	if entries := bag.Entries(); entries != nil {
		t.Errorf("nil.Entries() = %v, want nil", entries)
	}
	next := bag.With("k", "v")
	if next.Len() != 1 {
		t.Errorf("nil.With().Len = %d, want 1", next.Len())
	}
}

// TestWith_FromContext_RoundTrip ctx 往返
func TestWith_FromContext_RoundTrip(t *testing.T) {
	ctx := With(context.Background(), "tenant", "acme")
	v, ok := Get(ctx, "tenant")
	if !ok || v != "acme" {
		t.Fatalf("Get(tenant) = (%q,%v), want (acme,true)", v, ok)
	}
}

// TestWith_Accumulate 多次调用累积
func TestWith_Accumulate(t *testing.T) {
	ctx := With(context.Background(), "a", "1")
	ctx = With(ctx, "b", "2")
	ctx = With(ctx, "c", "3")
	bag := FromContext(ctx)
	if bag.Len() != 3 {
		t.Fatalf("Len = %d, want 3", bag.Len())
	}
	if v, _ := Get(ctx, "c"); v != "3" {
		t.Errorf("Get(c) = %q, want 3", v)
	}
}

// TestWith_Overwrite 同 Key 覆盖
func TestWith_Overwrite(t *testing.T) {
	ctx := With(context.Background(), "k", "v1")
	ctx = With(ctx, "k", "v2")
	if v, _ := Get(ctx, "k"); v != "v2" {
		t.Errorf("Get(k) = %q, want v2 (overwrite)", v)
	}
	bag := FromContext(ctx)
	if bag.Len() != 1 {
		t.Errorf("Len = %d, want 1 (no duplicate)", bag.Len())
	}
}

// TestWithEntries 批量注入
func TestWithEntries(t *testing.T) {
	ctx := WithEntries(context.Background(),
		Entry{Key: "a", Value: "1"},
		Entry{Key: "b", Value: "2"},
	)
	bag := FromContext(ctx)
	if bag.Len() != 2 {
		t.Fatalf("Len = %d, want 2", bag.Len())
	}
}

// TestGet_NotExist 不存在返回 false
func TestGet_NotExist(t *testing.T) {
	_, ok := Get(context.Background(), "missing")
	if ok {
		t.Error("Get on empty ctx should return false")
	}
}

// TestEncodeDecode_RoundTrip 编解码往返
func TestEncodeDecode_RoundTrip(t *testing.T) {
	original := NewBag().
		With("tenant.id", "acme").
		With("region", "cn-east-1").
		With("feature.flag", "beta")
	encoded := Encode(original)
	decoded := Decode(encoded)
	if decoded.Len() != original.Len() {
		t.Fatalf("Len mismatch: %d vs %d", decoded.Len(), original.Len())
	}
	for _, e := range original.Entries() {
		got, ok := decoded.Get(e.Key)
		if !ok || got != e.Value {
			t.Errorf("decoded.Get(%q) = (%q,%v), want (%q,true)", e.Key, got, ok, e.Value)
		}
	}
}

// TestEncode_SpecialChars 非 token 字符自动 percent-encode
func TestEncode_SpecialChars(t *testing.T) {
	bag := NewBag().With("key with space", "value=with=eq")
	encoded := Encode(bag)
	// 解码后应与原值一致
	decoded := Decode(encoded)
	v, ok := decoded.Get("key with space")
	if !ok || v != "value=with=eq" {
		t.Errorf("Get(key with space) = (%q,%v), want (value=with=eq,true)", v, ok)
	}
}

// TestEncode_NilEmpty 空 Bag 返回空字符串
func TestEncode_NilEmpty(t *testing.T) {
	if Encode(nil) != "" {
		t.Error("Encode(nil) should be empty")
	}
	if Encode(NewBag()) != "" {
		t.Error("Encode(empty bag) should be empty")
	}
}

// TestDecode_EmptyInvalid 空/非法输入容错
func TestDecode_EmptyInvalid(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"no_equals_sign",
		"=missing_key",
		",,",
	}
	for _, raw := range cases {
		if bag := Decode(raw); bag != nil && bag.Len() != 0 {
			t.Errorf("Decode(%q) = %+v, want nil or empty", raw, bag)
		}
	}
}

// TestDecode_PropertiesIgnored 忽略 properties 部分
func TestDecode_PropertiesIgnored(t *testing.T) {
	bag := Decode("tenant=acme;prop1=v1;prop2=v2")
	if bag == nil || bag.Len() != 1 {
		t.Fatalf("Decode(tenant=acme;props) Len = %d, want 1", func() int {
			if bag == nil {
				return 0
			}
			return bag.Len()
		}())
	}
	v, _ := bag.Get("tenant")
	if v != "acme" {
		t.Errorf("Get(tenant) = %q, want acme", v)
	}
}

// TestDecode_DuplicateKey 同 Key 后写覆盖
func TestDecode_DuplicateKey(t *testing.T) {
	bag := Decode("k=v1,k=v2")
	v, _ := bag.Get("k")
	if v != "v2" {
		t.Errorf("Get(k) = %q, want v2 (last write wins)", v)
	}
}

// TestInjectHTTP_ExtractHTTP_RoundTrip HTTP 往返
func TestInjectHTTP_ExtractHTTP_RoundTrip(t *testing.T) {
	ctx := WithEntries(context.Background(),
		Entry{Key: "tenant", Value: "acme"},
		Entry{Key: "region", Value: "cn-east-1"},
	)
	req, _ := http.NewRequest("GET", "/", nil)
	InjectHTTP(ctx, req.Header)

	if got := req.Header.Get(HeaderBaggage); got == "" {
		t.Fatal("Header[Baggage] should be set")
	}

	extracted := ExtractHTTP(context.Background(), req.Header)
	for _, e := range FromContext(ctx).Entries() {
		v, ok := Get(extracted, e.Key)
		if !ok || v != e.Value {
			t.Errorf("extracted.Get(%q) = (%q,%v), want (%q,true)", e.Key, v, ok, e.Value)
		}
	}
}

// TestInjectHTTP_NoBagNoOp ctx 无 Bag 时不动 hdr
func TestInjectHTTP_NoBagNoOp(t *testing.T) {
	hdr := http.Header{}
	hdr.Set(HeaderBaggage, "preset=should_remain")
	InjectHTTP(context.Background(), hdr)
	if got := hdr.Get(HeaderBaggage); got != "preset=should_remain" {
		t.Errorf("InjectHTTP with no bag should not touch hdr, got %q", got)
	}
}

// TestInjectHTTP_NilSafe nil 参数安全
func TestInjectHTTP_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InjectHTTP panicked on nil: %v", r)
		}
	}()
	InjectHTTP(context.Background(), nil)
}

// TestExtractHTTP_MergeWithExisting extract 后与已有 ctx Bag 合并
func TestExtractHTTP_MergeWithExisting(t *testing.T) {
	// ctx 已有 zeus.cluster
	ctx := With(context.Background(), "zeus.cluster", "canary")
	// 入站 Header 带 tenant
	hdr := http.Header{}
	hdr.Set(HeaderBaggage, "tenant=acme")

	merged := ExtractHTTP(ctx, hdr)
	if v, _ := Get(merged, "zeus.cluster"); v != "canary" {
		t.Errorf("Get(zeus.cluster) = %q, want canary (preserved)", v)
	}
	if v, _ := Get(merged, "tenant"); v != "acme" {
		t.Errorf("Get(tenant) = %q, want acme (extracted)", v)
	}
}

// TestExtractHTTP_NoHeaderReturnOrig hdr 无 baggage → 返回原 ctx
func TestExtractHTTP_NoHeaderReturnOrig(t *testing.T) {
	ctx := With(context.Background(), "preset", "value")
	hdr := http.Header{}
	out := ExtractHTTP(ctx, hdr)
	v, _ := Get(out, "preset")
	if v != "value" {
		t.Errorf("Get(preset) = %q, want value (ctx unchanged)", v)
	}
}

// TestHTTPMiddleware 入站自动 extract
func TestHTTPMiddleware(t *testing.T) {
	captured := make(chan context.Context, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Context()
	})
	h := HTTPMiddleware(next)

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderBaggage, "tenant=acme")
	h.ServeHTTP(nil, req)

	ctx := <-captured
	if v, _ := Get(ctx, "tenant"); v != "acme" {
		t.Errorf("after middleware, Get(tenant) = %q, want acme", v)
	}
}

// TestInjectMetadata_ExtractMetadata_RoundTrip gRPC metadata 往返
func TestInjectMetadata_ExtractMetadata_RoundTrip(t *testing.T) {
	ctx := WithEntries(context.Background(),
		Entry{Key: "tenant", Value: "acme"},
		Entry{Key: "region", Value: "cn-east-1"},
	)
	md := map[string]string{}
	InjectMetadata(ctx, md)
	if _, ok := md[MetadataBaggage]; !ok {
		t.Fatalf("md[%q] not set", MetadataBaggage)
	}

	extracted := ExtractMetadata(context.Background(), md)
	v, ok := Get(extracted, "tenant")
	if !ok || v != "acme" {
		t.Errorf("ExtractMetadata: Get(tenant) = (%q,%v), want (acme,true)", v, ok)
	}
}

// TestInjectMetadata_NoBagNoOp ctx 无 Bag 不动 md
func TestInjectMetadata_NoBagNoOp(t *testing.T) {
	md := map[string]string{"existing": "kept"}
	InjectMetadata(context.Background(), md)
	if v := md["existing"]; v != "kept" {
		t.Errorf("existing should be kept, got %q", v)
	}
	if _, ok := md[MetadataBaggage]; ok {
		t.Errorf("md[%q] should not be set when ctx has no bag", MetadataBaggage)
	}
}

// TestExtractMetadata_NilSafe nil md 安全
func TestExtractMetadata_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic: %v", r)
		}
	}()
	ctx := ExtractMetadata(context.Background(), nil)
	if ctx == nil {
		t.Error("should return non-nil ctx")
	}
}

// TestInjectMetadataMulti_ExtractMetadataMulti_RoundTrip 多值 metadata 往返
func TestInjectMetadataMulti_ExtractMetadataMulti_RoundTrip(t *testing.T) {
	ctx := With(context.Background(), "tenant", "acme")
	md := map[string][]string{}
	InjectMetadataMulti(ctx, md)

	if vals, ok := md[MetadataBaggage]; !ok || len(vals) != 1 {
		t.Fatalf("md[%q] not properly set: %v", MetadataBaggage, md[MetadataBaggage])
	}

	extracted := ExtractMetadataMulti(context.Background(), md)
	v, ok := Get(extracted, "tenant")
	if !ok || v != "acme" {
		t.Errorf("ExtractMetadataMulti: Get(tenant) = (%q,%v), want (acme,true)", v, ok)
	}
}

// TestExtractMetadataMulti_MultiValuesJoin 多值 metadata 按 "," 合并
func TestExtractMetadataMulti_MultiValuesJoin(t *testing.T) {
	md := map[string][]string{
		MetadataBaggage: {"tenant=acme", "region=cn"},
	}
	ctx := ExtractMetadataMulti(context.Background(), md)
	if v, _ := Get(ctx, "tenant"); v != "acme" {
		t.Errorf("Get(tenant) = %q, want acme", v)
	}
	if v, _ := Get(ctx, "region"); v != "cn" {
		t.Errorf("Get(region) = %q, want cn", v)
	}
}

// TestConstants 常量值
func TestConstants(t *testing.T) {
	if HeaderBaggage != "Baggage" {
		t.Errorf("HeaderBaggage = %q, want Baggage", HeaderBaggage)
	}
	if MetadataBaggage != "baggage" {
		t.Errorf("MetadataBaggage = %q, want baggage", MetadataBaggage)
	}
}

// TestTokenChar 边界字符判断
func TestTokenChar(t *testing.T) {
	tokenChars := "!#$%&'*+-.^_`|~abcXYZ09"
	for i := 0; i < len(tokenChars); i++ {
		if !isTokenChar(tokenChars[i]) {
			t.Errorf("char %q should be token", tokenChars[i])
		}
	}
	nonTokenChars := " ;/<>?@[]{}\""
	for i := 0; i < len(nonTokenChars); i++ {
		if isTokenChar(nonTokenChars[i]) {
			t.Errorf("char %q should NOT be token", nonTokenChars[i])
		}
	}
}
