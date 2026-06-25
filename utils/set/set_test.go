package set

import (
	"reflect"
	"sort"
	"testing"
)

// —— 辅助：忽略顺序的切片相等 ——

func sortedInts(s Set[int]) []int {
	out := s.Values()
	sort.Ints(out)
	return out
}

func sortedStrings(s Set[string]) []string {
	out := s.Values()
	sort.Strings(out)
	return out
}

// —— New / Size / IsEmpty ——

func TestNew_Empty(t *testing.T) {
	s := New[int]()
	if !s.IsEmpty() {
		t.Error("New should produce empty set")
	}
	if s.Size() != 0 {
		t.Errorf("Size = %d, want 0", s.Size())
	}
}

func TestNewWithCapacity(t *testing.T) {
	s := NewWithCapacity[int](100)
	if s.Size() != 0 {
		t.Errorf("Size = %d, want 0", s.Size())
	}
}

// —— Add ——

func TestAdd_NewElement(t *testing.T) {
	s := New[int]()
	if !s.Add(1) {
		t.Error("Add(1) on empty set should return true")
	}
	if s.Size() != 1 {
		t.Errorf("Size = %d, want 1", s.Size())
	}
}

func TestAdd_DuplicateElement(t *testing.T) {
	s := New[int]()
	s.Add(1)
	if s.Add(1) {
		t.Error("Add(1) twice should return false")
	}
	if s.Size() != 1 {
		t.Errorf("Size = %d, want 1", s.Size())
	}
}

// —— Remove ——

func TestRemove_Existing(t *testing.T) {
	s := FromSlice([]int{1, 2, 3})
	if !s.Remove(2) {
		t.Error("Remove(2) should return true")
	}
	if s.Contains(2) {
		t.Error("Set should not contain 2 after Remove")
	}
}

func TestRemove_NonExisting(t *testing.T) {
	s := FromSlice([]int{1, 2})
	if s.Remove(99) {
		t.Error("Remove(99) should return false")
	}
}

// —— Contains / Has ——

func TestContains(t *testing.T) {
	s := FromSlice([]string{"a", "b"})
	if !s.Contains("a") {
		t.Error("Should contain a")
	}
	if s.Contains("c") {
		t.Error("Should not contain c")
	}
	if !s.Has("b") {
		t.Error("Has(b) should be true (alias of Contains)")
	}
}

// —— FromSlice / Values ——

func TestFromSlice_Deduplicate(t *testing.T) {
	s := FromSlice([]int{1, 2, 2, 3, 3, 3})
	if s.Size() != 3 {
		t.Errorf("Size = %d, want 3 (auto-dedup)", s.Size())
	}
}

func TestValues_ReturnsAllElements(t *testing.T) {
	s := FromSlice([]int{1, 2, 3})
	got := s.Values()
	if len(got) != 3 {
		t.Fatalf("Values len = %d, want 3", len(got))
	}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("Values = %v, want [1,2,3]", got)
	}
}

// —— SortedValues ——

func TestSortedValues(t *testing.T) {
	s := FromSlice([]int{3, 1, 2})
	got := SortedValues(s, func(a, b int) bool { return a < b })
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("SortedValues = %v, want [1,2,3]", got)
	}
}

// —— Range ——

func TestRange_IteratesAll(t *testing.T) {
	s := FromSlice([]int{1, 2, 3, 4, 5})
	seen := make(map[int]bool)
	s.Range(func(v int) bool {
		seen[v] = true
		return true
	})
	if len(seen) != 5 {
		t.Errorf("Range visited %d items, want 5", len(seen))
	}
}

func TestRange_EarlyStop(t *testing.T) {
	s := FromSlice([]int{1, 2, 3, 4, 5})
	count := 0
	s.Range(func(v int) bool {
		count++
		return count < 3
	})
	if count != 3 {
		t.Errorf("Range should stop at 3, got %d", count)
	}
}

// —— Clone ——

func TestClone(t *testing.T) {
	s := FromSlice([]int{1, 2, 3})
	c := s.Clone()
	if !s.Equal(c) {
		t.Errorf("Clone not equal to original")
	}
	// 修改 clone 不应影响原 set
	c.Add(99)
	if s.Contains(99) {
		t.Error("Modifying clone affected original")
	}
}

// —— Clear ——

func TestClear(t *testing.T) {
	s := FromSlice([]int{1, 2, 3})
	s.Clear()
	if !s.IsEmpty() {
		t.Error("Set should be empty after Clear")
	}
}

// —— Union ——

func TestUnion(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{3, 4, 5})
	u := a.Union(b)
	got := sortedInts(u)
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("Union = %v, want [1,2,3,4,5]", got)
	}
}

func TestUnion_WithEmpty(t *testing.T) {
	a := FromSlice([]int{1, 2})
	b := New[int]()
	u := a.Union(b)
	if u.Size() != 2 {
		t.Errorf("Union with empty: Size = %d, want 2", u.Size())
	}
}

// —— Intersect ——

func TestIntersect(t *testing.T) {
	a := FromSlice([]int{1, 2, 3, 4})
	b := FromSlice([]int{3, 4, 5, 6})
	i := a.Intersect(b)
	got := sortedInts(i)
	if !reflect.DeepEqual(got, []int{3, 4}) {
		t.Errorf("Intersect = %v, want [3,4]", got)
	}
}

func TestIntersect_Disjoint(t *testing.T) {
	a := FromSlice([]int{1, 2})
	b := FromSlice([]int{3, 4})
	i := a.Intersect(b)
	if !i.IsEmpty() {
		t.Errorf("Intersect of disjoint sets should be empty, got %v", i.Values())
	}
}

// —— Difference ——

func TestDifference(t *testing.T) {
	a := FromSlice([]int{1, 2, 3, 4})
	b := FromSlice([]int{3, 4, 5})
	d := a.Difference(b)
	got := sortedInts(d)
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("Difference = %v, want [1,2]", got)
	}
}

// —— SymmetricDifference ——

func TestSymmetricDifference(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{2, 3, 4})
	sd := a.SymmetricDifference(b)
	got := sortedInts(sd)
	if !reflect.DeepEqual(got, []int{1, 4}) {
		t.Errorf("SymmetricDifference = %v, want [1,4]", got)
	}
}

// —— IsSubset / IsSuperset ——

func TestIsSubset_True(t *testing.T) {
	a := FromSlice([]int{1, 2})
	b := FromSlice([]int{1, 2, 3})
	if !a.IsSubset(b) {
		t.Error("{1,2} should be subset of {1,2,3}")
	}
}

func TestIsSubset_False(t *testing.T) {
	a := FromSlice([]int{1, 4})
	b := FromSlice([]int{1, 2, 3})
	if a.IsSubset(b) {
		t.Error("{1,4} should NOT be subset of {1,2,3}")
	}
}

func TestIsSubset_EqualSets(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{1, 2, 3})
	if !a.IsSubset(b) {
		t.Error("Equal sets should be subset (reflexive)")
	}
}

func TestIsSuperset(t *testing.T) {
	a := FromSlice([]int{1, 2, 3, 4})
	b := FromSlice([]int{2, 3})
	if !a.IsSuperset(b) {
		t.Error("{1,2,3,4} should be superset of {2,3}")
	}
}

// —— IsDisjoint ——

func TestIsDisjoint_True(t *testing.T) {
	a := FromSlice([]int{1, 2})
	b := FromSlice([]int{3, 4})
	if !a.IsDisjoint(b) {
		t.Error("{1,2} and {3,4} should be disjoint")
	}
}

func TestIsDisjoint_False(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{3, 4})
	if a.IsDisjoint(b) {
		t.Error("{1,2,3} and {3,4} should NOT be disjoint")
	}
}

// —— Equal ——

func TestEqual_SameElements(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{3, 2, 1})
	if !a.Equal(b) {
		t.Error("{1,2,3} should equal {3,2,1}")
	}
}

func TestEqual_DifferentSizes(t *testing.T) {
	a := FromSlice([]int{1, 2})
	b := FromSlice([]int{1, 2, 3})
	if a.Equal(b) {
		t.Error("{1,2} and {1,2,3} should not be equal")
	}
}

func TestEqual_DifferentElements(t *testing.T) {
	a := FromSlice([]int{1, 2, 3})
	b := FromSlice([]int{1, 2, 4})
	if a.Equal(b) {
		t.Error("{1,2,3} and {1,2,4} should not be equal")
	}
}

// —— String type ——

func TestSet_StringType(t *testing.T) {
	s := FromSlice([]string{"alpha", "beta", "alpha"})
	if s.Size() != 2 {
		t.Errorf("Size = %d, want 2", s.Size())
	}
	got := sortedStrings(s)
	if !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Errorf("Values = %v, want [alpha,beta]", got)
	}
}

// —— Custom struct type ——

func TestSet_CustomType(t *testing.T) {
	type User struct{ ID int }
	s := New[User]()
	s.Add(User{ID: 1})
	s.Add(User{ID: 2})
	s.Add(User{ID: 1}) // duplicate
	if s.Size() != 2 {
		t.Errorf("Size = %d, want 2 (auto-dedup by struct value)", s.Size())
	}
	if !s.Contains(User{ID: 1}) {
		t.Error("Should contain User{ID:1}")
	}
}

// —— 集合代数不变量 ——

func TestAlgebraIdentityUnion(t *testing.T) {
	// A ∪ ∅ = A
	a := FromSlice([]int{1, 2, 3})
	empty := New[int]()
	if !a.Union(empty).Equal(a) {
		t.Error("A ∪ ∅ should equal A")
	}
}

func TestAlgebraIdentityIntersect(t *testing.T) {
	// A ∩ A = A
	a := FromSlice([]int{1, 2, 3})
	if !a.Intersect(a).Equal(a) {
		t.Error("A ∩ A should equal A")
	}
}

func TestAlgebraDifferenceAndIntersect(t *testing.T) {
	// (A - B) ∩ B = ∅
	a := FromSlice([]int{1, 2, 3, 4})
	b := FromSlice([]int{3, 4, 5})
	diff := a.Difference(b)
	if !diff.Intersect(b).IsEmpty() {
		t.Error("(A-B) ∩ B should be empty")
	}
}

// —— Benchmark ——

func BenchmarkAdd(b *testing.B) {
	s := New[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Add(i)
	}
}

func BenchmarkContains(b *testing.B) {
	s := NewWithCapacity[int](1000)
	for i := 0; i < 1000; i++ {
		s.Add(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Contains(i % 1000)
	}
}

func BenchmarkUnion(b *testing.B) {
	a := NewWithCapacity[int](100)
	c := NewWithCapacity[int](100)
	for i := 0; i < 100; i++ {
		a.Add(i)
		c.Add(i + 50)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Union(c)
	}
}

func BenchmarkIntersect(b *testing.B) {
	a := NewWithCapacity[int](1000)
	c := NewWithCapacity[int](1000)
	for i := 0; i < 1000; i++ {
		a.Add(i)
		c.Add(i + 500)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Intersect(c)
	}
}

func BenchmarkFromSlice(b *testing.B) {
	items := make([]int, 1000)
	for i := range items {
		items[i] = i % 200 // 制造重复
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FromSlice(items)
	}
}
