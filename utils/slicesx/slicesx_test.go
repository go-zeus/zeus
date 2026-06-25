package slicesx

import (
	"reflect"
	"testing"
)

// —— Map ——

func TestMap(t *testing.T) {
	in := []int{1, 2, 3}
	got := Map(in, func(n int) int { return n * 2 })
	want := []int{2, 4, 6}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Map = %v, want %v", got, want)
	}
}

func TestMap_TypeTransform(t *testing.T) {
	in := []int{1, 2, 3}
	got := Map(in, func(n int) string { return "x" })
	want := []string{"x", "x", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Map = %v, want %v", got, want)
	}
}

func TestMap_Empty(t *testing.T) {
	got := Map([]int{}, func(n int) int { return n })
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestMap_Nil(t *testing.T) {
	var in []int = nil
	got := Map(in, func(n int) int { return n })
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// —— Filter ——

func TestFilter(t *testing.T) {
	in := []int{1, 2, 3, 4, 5}
	got := Filter(in, func(n int) bool { return n%2 == 0 })
	want := []int{2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Filter = %v, want %v", got, want)
	}
}

func TestFilter_AllMatched(t *testing.T) {
	in := []int{1, 2, 3}
	got := Filter(in, func(n int) bool { return true })
	if !reflect.DeepEqual(got, in) {
		t.Errorf("Filter = %v, want %v", got, in)
	}
}

func TestFilter_NoneMatched(t *testing.T) {
	in := []int{1, 2, 3}
	got := Filter(in, func(n int) bool { return false })
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilter_Nil(t *testing.T) {
	var in []int = nil
	got := Filter(in, func(n int) bool { return true })
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// —— Reduce ——

func TestReduce_Sum(t *testing.T) {
	in := []int{1, 2, 3, 4}
	got := Reduce(in, 0, func(acc, n int) int { return acc + n })
	if got != 10 {
		t.Errorf("Reduce = %d, want 10", got)
	}
}

func TestReduce_TypeTransform(t *testing.T) {
	in := []string{"a", "bb", "ccc"}
	got := Reduce(in, 0, func(acc int, s string) int { return acc + len(s) })
	if got != 6 {
		t.Errorf("Reduce = %d, want 6", got)
	}
}

func TestReduce_Empty(t *testing.T) {
	got := Reduce([]int{}, 42, func(acc, n int) int { return acc + n })
	if got != 42 {
		t.Errorf("Reduce on empty = %d, want 42 (init)", got)
	}
}

// —— Contains / ContainsFunc ——

func TestContains_Found(t *testing.T) {
	in := []int{1, 2, 3}
	if !Contains(in, 2) {
		t.Error("expected Contains(in, 2) = true")
	}
}

func TestContains_NotFound(t *testing.T) {
	in := []int{1, 2, 3}
	if Contains(in, 99) {
		t.Error("expected Contains(in, 99) = false")
	}
}

func TestContainsFunc(t *testing.T) {
	in := []string{"alice", "bob"}
	if !ContainsFunc(in, func(s string) bool { return s == "bob" }) {
		t.Error("ContainsFunc should find bob")
	}
	if ContainsFunc(in, func(s string) bool { return s == "carol" }) {
		t.Error("ContainsFunc should not find carol")
	}
}

// —— Find / FindLast ——

func TestFind(t *testing.T) {
	type User struct{ ID int }
	in := []User{{1}, {2}, {3}}
	v, idx := Find(in, func(u User) bool { return u.ID == 2 })
	if idx != 1 || v.ID != 2 {
		t.Errorf("Find = (v=%v, idx=%d), want ({2}, 1)", v, idx)
	}
}

func TestFind_NotFound(t *testing.T) {
	in := []int{1, 2, 3}
	v, idx := Find(in, func(n int) bool { return n == 99 })
	if idx != -1 || v != 0 {
		t.Errorf("Find = (v=%d, idx=%d), want (0, -1)", v, idx)
	}
}

func TestFindLast(t *testing.T) {
	in := []int{1, 2, 1, 3, 1}
	v, idx := FindLast(in, func(n int) bool { return n == 1 })
	if idx != 4 || v != 1 {
		t.Errorf("FindLast = (v=%d, idx=%d), want (1, 4)", v, idx)
	}
}

// —— Unique / UniqueBy ——

func TestUnique(t *testing.T) {
	in := []int{1, 2, 2, 3, 1, 4}
	got := Unique(in)
	want := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Unique = %v, want %v", got, want)
	}
}

func TestUnique_Empty(t *testing.T) {
	got := Unique([]int{})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestUnique_PreservesOrder(t *testing.T) {
	in := []string{"banana", "apple", "banana", "cherry"}
	got := Unique(in)
	want := []string{"banana", "apple", "cherry"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Unique order = %v, want %v", got, want)
	}
}

func TestUniqueBy(t *testing.T) {
	type User struct{ ID int }
	in := []User{{1}, {2}, {1}, {3}}
	got := UniqueBy(in, func(u User) int { return u.ID })
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Errorf("UniqueBy = %v", got)
	}
}

// —— GroupBy ——

func TestGroupBy(t *testing.T) {
	type User struct{ Team, Name string }
	in := []User{
		{"A", "alice"},
		{"B", "bob"},
		{"A", "carol"},
	}
	got := GroupBy(in, func(u User) string { return u.Team })
	if len(got) != 2 {
		t.Errorf("GroupBy len = %d, want 2", len(got))
	}
	if len(got["A"]) != 2 || got["A"][0].Name != "alice" || got["A"][1].Name != "carol" {
		t.Errorf("GroupBy A = %v", got["A"])
	}
	if len(got["B"]) != 1 || got["B"][0].Name != "bob" {
		t.Errorf("GroupBy B = %v", got["B"])
	}
}

func TestGroupBy_Empty(t *testing.T) {
	got := GroupBy([]int{}, func(n int) int { return n })
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// —— Chunk ——

func TestChunk(t *testing.T) {
	in := []int{1, 2, 3, 4, 5}
	got := Chunk(in, 2)
	want := [][]int{{1, 2}, {3, 4}, {5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chunk = %v, want %v", got, want)
	}
}

func TestChunk_ExactMultiple(t *testing.T) {
	in := []int{1, 2, 3, 4}
	got := Chunk(in, 2)
	want := [][]int{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chunk = %v, want %v", got, want)
	}
}

func TestChunk_SizeLargerThanSlice(t *testing.T) {
	in := []int{1, 2}
	got := Chunk(in, 5)
	want := [][]int{{1, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chunk = %v, want %v", got, want)
	}
}

func TestChunk_InvalidSize(t *testing.T) {
	if got := Chunk([]int{1, 2, 3}, 0); got != nil {
		t.Errorf("Chunk(_, 0) = %v, want nil", got)
	}
	if got := Chunk([]int{1, 2, 3}, -1); got != nil {
		t.Errorf("Chunk(_, -1) = %v, want nil", got)
	}
}

// —— Flat ——

func TestFlat(t *testing.T) {
	in := [][]int{{1, 2}, {3, 4}, {5}}
	got := Flat(in)
	want := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Flat = %v, want %v", got, want)
	}
}

func TestFlat_WithEmptySubs(t *testing.T) {
	in := [][]int{{1}, {}, {2, 3}, {}}
	got := Flat(in)
	want := []int{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Flat = %v, want %v", got, want)
	}
}

func TestFlat_Empty(t *testing.T) {
	got := Flat([][]int{})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// —— Reverse ——

func TestReverse(t *testing.T) {
	in := []int{1, 2, 3}
	got := Reverse(in)
	want := []int{3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Reverse = %v, want %v", got, want)
	}
}

// TestReverse_DoesNotMutateOriginal 入参不应被修改
func TestReverse_DoesNotMutateOriginal(t *testing.T) {
	in := []int{1, 2, 3}
	_ = Reverse(in)
	if !reflect.DeepEqual(in, []int{1, 2, 3}) {
		t.Errorf("original was mutated: %v", in)
	}
}

// —— AnyMatch / AllMatch / NoneMatch ——

func TestAnyMatch(t *testing.T) {
	in := []int{1, 3, 5}
	if !AnyMatch(in, func(n int) bool { return n == 3 }) {
		t.Error("AnyMatch(_, 3) should be true")
	}
	if AnyMatch(in, func(n int) bool { return n == 99 }) {
		t.Error("AnyMatch(_, 99) should be false")
	}
}

func TestAnyMatch_Empty(t *testing.T) {
	if AnyMatch([]int{}, func(n int) bool { return true }) {
		t.Error("AnyMatch on empty should be false")
	}
}

func TestAllMatch(t *testing.T) {
	in := []int{2, 4, 6}
	if !AllMatch(in, func(n int) bool { return n%2 == 0 }) {
		t.Error("AllMatch should be true for all even")
	}
	if AllMatch(in, func(n int) bool { return n > 5 }) {
		t.Error("AllMatch should be false (5 is not >5)")
	}
}

func TestAllMatch_Empty(t *testing.T) {
	if !AllMatch([]int{}, func(n int) bool { return true }) {
		t.Error("AllMatch on empty should be true (vacuous truth)")
	}
}

func TestNoneMatch(t *testing.T) {
	in := []int{1, 3, 5}
	if !NoneMatch(in, func(n int) bool { return n%2 == 0 }) {
		t.Error("NoneMatch on odd-only should be true for 'is even'")
	}
	if NoneMatch(in, func(n int) bool { return n == 1 }) {
		t.Error("NoneMatch should be false when 1 exists")
	}
}

// —— Benchmark ——

func BenchmarkMap(b *testing.B) {
	in := make([]int, 1000)
	for i := range in {
		in[i] = i
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Map(in, func(n int) int { return n * 2 })
	}
}

func BenchmarkFilter(b *testing.B) {
	in := make([]int, 1000)
	for i := range in {
		in[i] = i
	}
	pred := func(n int) bool { return n%2 == 0 }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Filter(in, pred)
	}
}

func BenchmarkUnique(b *testing.B) {
	in := make([]int, 500)
	for i := range in {
		in[i] = i % 100 // 制造重复
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Unique(in)
	}
}
