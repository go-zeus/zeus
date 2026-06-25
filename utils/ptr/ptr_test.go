package ptr

import (
	"reflect"
	"testing"
	"time"
)

// —— Of ——

func TestOf_Int(t *testing.T) {
	p := Of(42)
	if p == nil {
		t.Fatal("Of returned nil")
	}
	if *p != 42 {
		t.Errorf("*p = %d, want 42", *p)
	}
}

func TestOf_String(t *testing.T) {
	p := Of("hello")
	if *p != "hello" {
		t.Errorf("*p = %q, want hello", *p)
	}
}

func TestOf_Struct(t *testing.T) {
	type User struct{ ID int }
	p := Of(User{ID: 7})
	if p.ID != 7 {
		t.Errorf("p.ID = %d, want 7", p.ID)
	}
}

func TestOf_ReturnsDistinctPointer(t *testing.T) {
	// 每次调用 Of 应产生独立指针（不是同一地址）
	p1 := Of(1)
	p2 := Of(1)
	if p1 == p2 {
		t.Error("Of should return distinct pointer per call")
	}
}

// —— Deref ——

func TestDeref_NilReturnsDefault(t *testing.T) {
	var p *int
	got := Deref(p, 99)
	if got != 99 {
		t.Errorf("Deref(nil, 99) = %d, want 99", got)
	}
}

func TestDeref_NonNilReturnsValue(t *testing.T) {
	p := Of(42)
	got := Deref(p, 99)
	if got != 42 {
		t.Errorf("Deref(p, 99) = %d, want 42", got)
	}
}

func TestDeref_Struct(t *testing.T) {
	type Config struct{ Timeout time.Duration }
	var p *Config
	got := Deref(p, Config{Timeout: time.Second})
	if got.Timeout != time.Second {
		t.Errorf("Deref nil Config Timeout = %v, want 1s", got.Timeout)
	}
}

// —— Equal ——

func TestEqual_BothNil(t *testing.T) {
	var a, b *int
	if !Equal(a, b) {
		t.Error("Equal(nil, nil) should be true")
	}
}

func TestEqual_OneNil(t *testing.T) {
	a := Of(1)
	var b *int
	if Equal(a, b) {
		t.Error("Equal(p, nil) should be false")
	}
	if Equal(b, a) {
		t.Error("Equal(nil, p) should be false")
	}
}

func TestEqual_SameValue(t *testing.T) {
	a := Of(42)
	b := Of(42)
	if !Equal(a, b) {
		t.Error("Equal(42, 42) should be true")
	}
}

func TestEqual_DiffValue(t *testing.T) {
	a := Of(1)
	b := Of(2)
	if Equal(a, b) {
		t.Error("Equal(1, 2) should be false")
	}
}

// —— DeepEqual ——

func TestDeepEqual_BothNil(t *testing.T) {
	var a, b *map[string]int
	if !DeepEqual(a, b) {
		t.Error("DeepEqual(nil, nil) should be true")
	}
}

func TestDeepEqual_OneNil(t *testing.T) {
	a := Of(map[string]int{"x": 1})
	var b *map[string]int
	if DeepEqual(a, b) {
		t.Error("DeepEqual(p, nil) should be false")
	}
}

func TestDeepEqual_MapSameValue(t *testing.T) {
	a := Of(map[string]int{"x": 1, "y": 2})
	b := Of(map[string]int{"y": 2, "x": 1})
	if !DeepEqual(a, b) {
		t.Error("DeepEqual should be true for same map contents")
	}
}

func TestDeepEqual_MapDiffValue(t *testing.T) {
	a := Of(map[string]int{"x": 1})
	b := Of(map[string]int{"x": 2})
	if DeepEqual(a, b) {
		t.Error("DeepEqual should be false for different maps")
	}
}

func TestDeepEqual_SliceSameValue(t *testing.T) {
	a := Of([]int{1, 2, 3})
	b := Of([]int{1, 2, 3})
	if !DeepEqual(a, b) {
		t.Error("DeepEqual should be true for same slice contents")
	}
}

// —— IsNil ——

func TestIsNil(t *testing.T) {
	var p *int
	if !IsNil(p) {
		t.Error("IsNil(nil) should be true")
	}
	if IsNil(Of(1)) {
		t.Error("IsNil(non-nil) should be false")
	}
}

// —— Clone ——

func TestClone_NilReturnsNil(t *testing.T) {
	var p *int
	if Clone(p) != nil {
		t.Error("Clone(nil) should return nil")
	}
}

func TestClone_DistinctPointer(t *testing.T) {
	type User struct{ ID int }
	orig := Of(User{ID: 1})
	clone := Clone(orig)
	if clone == orig {
		t.Error("Clone should return distinct pointer")
	}
	if clone.ID != orig.ID {
		t.Errorf("Clone value = %v, want %v", clone.ID, orig.ID)
	}
}

func TestClone_IndependentMutation(t *testing.T) {
	orig := Of(42)
	clone := Clone(orig)
	*clone = 99
	if *orig != 42 {
		t.Errorf("modifying clone affected orig: %d", *orig)
	}
}

// —— 综合场景 ——

func TestCompositeUsage(t *testing.T) {
	// 模拟可选配置场景：用 *Duration 表达"未设置"
	type Config struct {
		Timeout *time.Duration
		Retries *int
	}
	def := Config{
		Timeout: Of(10 * time.Second),
		// Retries 故意留 nil
	}

	// Deref 应用默认值
	timeout := Deref(def.Timeout, time.Minute)
	retries := Deref(def.Retries, 3)

	if timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", timeout)
	}
	if retries != 3 {
		t.Errorf("retries = %d, want 3", retries)
	}
}

func TestTypeIntegration(t *testing.T) {
	// 用 reflect 验证 Of 返回的类型
	p := Of("hello")
	if reflect.TypeOf(p).Kind() != reflect.Ptr {
		t.Errorf("Of should return pointer, got %v", reflect.TypeOf(p).Kind())
	}
	if reflect.TypeOf(p).Elem().Kind() != reflect.String {
		t.Errorf("pointer elem should be string")
	}
}

// —— Benchmark ——

func BenchmarkOf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Of(i)
	}
}

func BenchmarkDeref_Nil(b *testing.B) {
	var p *int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Deref(p, 0)
	}
}

func BenchmarkDeref_NonNil(b *testing.B) {
	p := Of(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Deref(p, 0)
	}
}

func BenchmarkEqual(b *testing.B) {
	a := Of(1)
	c := Of(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Equal(a, c)
	}
}

func BenchmarkClone(b *testing.B) {
	orig := Of(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Clone(orig)
	}
}
