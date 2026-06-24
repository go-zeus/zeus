package metadata

import (
	"context"
	"reflect"
	"testing"
)

func TestMDSet(t *testing.T) {
	ctx := Set(context.TODO(), "Key", "val")

	val, ok := Get(ctx, "Key")
	if !ok {
		t.Fatal("key Key not found")
	}
	if val != "val" {
		t.Errorf("key Key with value val != %v", val)
	}
}

func TestMDDelete(t *testing.T) {
	md := MD{
		"Foo": "bar",
		"Baz": "empty",
	}

	ctx := NewContext(context.TODO(), md)
	ctx = Delete(ctx, "Baz")

	emd, ok := FromContext(ctx)
	if !ok {
		t.Fatal("key Key not found")
	}

	_, ok = emd["Baz"]
	if ok {
		t.Fatal("key Baz not deleted")
	}

}

func TestMDCopy(t *testing.T) {
	md := MD{
		"Foo": "bar",
		"bar": "baz",
	}

	cp := Copy(md)

	for k, v := range md {
		if cv := cp[k]; cv != v {
			t.Fatalf("Got %s:%s for %s:%s", k, cv, k, v)
		}
	}
}

func TestMDContext(t *testing.T) {
	md := MD{
		"Foo": "bar",
	}

	ctx := NewContext(context.TODO(), md)

	emd, ok := FromContext(ctx)
	if !ok {
		t.Errorf("Unexpected error retrieving MD, got %t", ok)
	}

	if emd["Foo"] != md["Foo"] {
		t.Errorf("Expected key: %s val: %s, got key: %s val: %s", "Foo", md["Foo"], "Foo", emd["Foo"])
	}

	if i := len(emd); i != 1 {
		t.Errorf("Expected MD length 1 got %d", i)
	}
}

func TestMergeContext(t *testing.T) {
	type args struct {
		existing  MD
		append    MD
		overwrite bool
	}
	tests := []struct {
		name string
		args args
		want MD
	}{
		{
			name: "matching key, overwrite false",
			args: args{
				existing:  MD{"Foo": "bar", "Sumo": "demo"},
				append:    MD{"Sumo": "demo2"},
				overwrite: false,
			},
			want: MD{"Foo": "bar", "Sumo": "demo"},
		},
		{
			name: "matching key, overwrite true",
			args: args{
				existing:  MD{"Foo": "bar", "Sumo": "demo"},
				append:    MD{"Sumo": "demo2"},
				overwrite: true,
			},
			want: MD{"Foo": "bar", "Sumo": "demo2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := FromContext(MergeContext(NewContext(context.TODO(), tt.args.existing), tt.args.append, tt.args.overwrite)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---- 新增测试 ----

// TestMD_Set_Get 直接在 MD 上 Set 和 Get
func TestMD_Set_Get(t *testing.T) {
	md := MD{}
	md.Set("key1", "value1")
	md.Set("key2", "value2")

	val, ok := md.Get("key1")
	if !ok {
		t.Fatal("key1 not found")
	}
	if val != "value1" {
		t.Errorf("Get(key1) = %q, want %q", val, "value1")
	}

	val, ok = md.Get("key2")
	if !ok {
		t.Fatal("key2 not found")
	}
	if val != "value2" {
		t.Errorf("Get(key2) = %q, want %q", val, "value2")
	}

	// 获取不存在的 key
	_, ok = md.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

// TestMD_Delete 直接在 MD 上删除 key
func TestMD_Delete(t *testing.T) {
	md := MD{"foo": "bar", "baz": "qux"}
	md.Delete("foo")

	_, ok := md.Get("foo")
	if ok {
		t.Error("foo should be deleted")
	}

	val, ok := md.Get("baz")
	if !ok || val != "qux" {
		t.Errorf("baz should still exist, got ok=%v val=%q", ok, val)
	}

	// 删除不存在的 key 不应 panic
	md.Delete("nonexistent")
}

// TestMD_Iterate 遍历所有 key
func TestMD_Iterate(t *testing.T) {
	md := MD{"a": "1", "b": "2", "c": "3"}
	seen := make(map[string]string)
	for k, v := range md {
		seen[k] = v
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 keys, got %d", len(seen))
	}
	if seen["a"] != "1" || seen["b"] != "2" || seen["c"] != "3" {
		t.Errorf("iterate values mismatch: %v", seen)
	}
}

// TestMD_New 从键值对创建 MD 并绑定到 context
func TestMD_New(t *testing.T) {
	md := MD{"k1": "v1", "k2": "v2"}
	ctx := NewContext(context.TODO(), md)

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext should return ok=true")
	}
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
	if got["k1"] != "v1" || got["k2"] != "v2" {
		t.Errorf("FromContext values mismatch: %v", got)
	}
}

// TestMD_Clone 克隆元数据
func TestMD_Clone(t *testing.T) {
	md := MD{"x": "10", "y": "20"}
	cp := Copy(md)

	// 修改副本不影响原始
	cp["x"] = "changed"
	if md["x"] != "10" {
		t.Error("modifying clone should not affect original")
	}

	// 原始修改不影响副本
	md["y"] = "changed"
	if cp["y"] != "20" {
		t.Error("modifying original should not affect clone")
	}
}

// TestMD_Merge 合并两个元数据实例
func TestMD_Merge(t *testing.T) {
	existing := MD{"a": "1", "b": "2"}
	patch := MD{"b": "3", "c": "4"}

	// 不覆盖
	merged, _ := FromContext(MergeContext(NewContext(context.TODO(), existing), patch, false))
	if merged["b"] != "2" {
		t.Errorf("overwrite=false: b should stay '2', got %q", merged["b"])
	}
	if merged["c"] != "4" {
		t.Errorf("overwrite=false: c should be '4', got %q", merged["c"])
	}

	// 覆盖
	merged2, _ := FromContext(MergeContext(NewContext(context.TODO(), existing), patch, true))
	if merged2["b"] != "3" {
		t.Errorf("overwrite=true: b should become '3', got %q", merged2["b"])
	}
}

// TestMD_Empty 空元数据操作
func TestMD_Empty(t *testing.T) {
	md := MD{}

	// Get 不存在的 key
	_, ok := md.Get("nothing")
	if ok {
		t.Error("Get on empty MD should return false")
	}

	// Delete 不存在的 key 不 panic
	md.Delete("nothing")

	// Copy 空 MD
	cp := Copy(md)
	if len(cp) != 0 {
		t.Errorf("Copy of empty MD should be empty, got %d keys", len(cp))
	}

	// FromContext 无元数据的 context
	_, ok = FromContext(context.TODO())
	if ok {
		t.Error("FromContext on context without metadata should return false")
	}

	// Get 从无元数据的 context
	_, ok = Get(context.TODO(), "key")
	if ok {
		t.Error("Get on context without metadata should return false")
	}
}

// TestMD_Equal 测试 Equal 方法
func TestMD_Equal(t *testing.T) {
	tests := []struct {
		name string
		a    MD
		b    MD
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil b not", nil, MD{}, false},
		{"b nil a not", MD{}, nil, false},
		{"both empty", MD{}, MD{}, true},
		{"same content", MD{"k": "v"}, MD{"k": "v"}, true},
		{"different value", MD{"k": "v1"}, MD{"k": "v2"}, false},
		{"different keys", MD{"a": "1"}, MD{"b": "1"}, false},
		{"different length", MD{"a": "1"}, MD{"a": "1", "b": "2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMD_MergeContext_NilCtx 合并时 ctx 为 nil
func TestMD_MergeContext_NilCtx(t *testing.T) {
	patch := MD{"key": "val"}
	ctx := MergeContext(context.TODO(), patch, true)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext should return ok=true")
	}
	if got["key"] != "val" {
		t.Errorf("MergeContext with nil ctx: key = %q, want %q", got["key"], "val")
	}
}

// TestMD_MergeContext_DeleteByEmptyValue 合并时空值表示删除
func TestMD_MergeContext_DeleteByEmptyValue(t *testing.T) {
	existing := MD{"a": "1", "b": "2"}
	patch := MD{"b": ""} // 空值表示删除
	merged, _ := FromContext(MergeContext(NewContext(context.TODO(), existing), patch, true))
	if _, ok := merged["b"]; ok {
		t.Error("empty value in patch should delete key b")
	}
	if merged["a"] != "1" {
		t.Errorf("a should remain '1', got %q", merged["a"])
	}
}

// TestMD_Set_Delete_Context 通过 context 的 Set/Delete 操作
func TestMD_Set_Delete_Context(t *testing.T) {
	ctx := context.TODO()

	// Set
	ctx = Set(ctx, "k1", "v1")
	val, ok := Get(ctx, "k1")
	if !ok || val != "v1" {
		t.Errorf("Get after Set: ok=%v val=%q, want ok=true val='v1'", ok, val)
	}

	// Delete (通过设置空值)
	ctx = Delete(ctx, "k1")
	_, ok = Get(ctx, "k1")
	if ok {
		t.Error("Get after Delete should return false")
	}
}
