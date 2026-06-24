package page

import (
	"strings"
	"testing"
)

// —— Request.Normalize ——

func TestRequest_Normalize_Defaults(t *testing.T) {
	r := Request{}.Normalize()
	if r.Page != 1 {
		t.Errorf("Page = %d, want 1", r.Page)
	}
	if r.Size != 20 {
		t.Errorf("Size = %d, want 20", r.Size)
	}
	if r.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", r.MaxSize)
	}
}

func TestRequest_Normalize_NegativePage(t *testing.T) {
	r := Request{Page: -5, Size: 10}.Normalize()
	if r.Page != 1 {
		t.Errorf("Page = %d, want 1", r.Page)
	}
}

func TestRequest_Normalize_SizeClampedToMaxSize(t *testing.T) {
	r := Request{Size: 500, MaxSize: 100}.Normalize()
	if r.Size != 100 {
		t.Errorf("Size = %d, want 100 (clamped)", r.Size)
	}
}

func TestRequest_Normalize_CustomMaxSize(t *testing.T) {
	r := Request{Size: 50, MaxSize: 30}.Normalize()
	if r.Size != 30 {
		t.Errorf("Size = %d, want 30", r.Size)
	}
}

// —— Offset / Limit ——

func TestRequest_Offset(t *testing.T) {
	r := Request{Page: 3, Size: 20}.Normalize()
	if r.Offset() != 40 {
		t.Errorf("Offset = %d, want 40", r.Offset())
	}
	if r.Limit() != 20 {
		t.Errorf("Limit = %d, want 20", r.Limit())
	}
}

func TestRequest_Offset_FirstPage(t *testing.T) {
	r := Request{Page: 1, Size: 20}.Normalize()
	if r.Offset() != 0 {
		t.Errorf("Offset = %d, want 0", r.Offset())
	}
}

// —— ParseSort ——

func TestParseSort_Empty(t *testing.T) {
	if fields := ParseSort(""); len(fields) != 0 {
		t.Errorf("expected empty, got %v", fields)
	}
}

func TestParseSort_SingleField(t *testing.T) {
	fields := ParseSort("created_at:desc")
	if len(fields) != 1 {
		t.Fatalf("len = %d", len(fields))
	}
	if fields[0].Name != "created_at" || !fields[0].Desc {
		t.Errorf("field = %+v", fields[0])
	}
}

func TestParseSort_MultipleFields(t *testing.T) {
	fields := ParseSort("a:asc, b:desc, c")
	if len(fields) != 3 {
		t.Fatalf("len = %d", len(fields))
	}
	if fields[0].Name != "a" || fields[0].Desc {
		t.Errorf("field 0 = %+v", fields[0])
	}
	if fields[1].Name != "b" || !fields[1].Desc {
		t.Errorf("field 1 = %+v", fields[1])
	}
	if fields[2].Name != "c" || fields[2].Desc {
		t.Errorf("field 2 = %+v", fields[2])
	}
}

func TestParseSort_IgnoresEmptyAndJunk(t *testing.T) {
	fields := ParseSort(", , a:invalid, b:desc,")
	if len(fields) != 2 {
		t.Fatalf("len = %d", len(fields))
	}
	// "a:invalid" 中 invalid 不是 desc，按默认 asc 处理
	if fields[0].Name != "a" || fields[0].Desc {
		t.Errorf("field 0 = %+v", fields[0])
	}
}

func TestSortField_String(t *testing.T) {
	f := SortField{Name: "x", Desc: true}
	if f.String() != "x:desc" {
		t.Errorf("got %q", f.String())
	}
	f2 := SortField{Name: "x", Desc: false}
	if f2.String() != "x:asc" {
		t.Errorf("got %q", f2.String())
	}
}

// —— Paginate ——

func TestPaginate_BasicSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	req := Request{Page: 2, Size: 3}
	resp := Paginate(req, items)

	if len(resp.Items) != 3 {
		t.Fatalf("len = %d, want 3", len(resp.Items))
	}
	if resp.Items[0] != 4 || resp.Items[2] != 6 {
		t.Errorf("items = %v", resp.Items)
	}
	if resp.Total != 10 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.TotalPages != 4 {
		t.Errorf("TotalPages = %d, want 4 (ceil(10/3))", resp.TotalPages)
	}
	if !resp.HasPrev {
		t.Error("should have prev")
	}
	if !resp.HasNext {
		t.Error("should have next")
	}
}

func TestPaginate_FirstPage(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	resp := Paginate(Request{Page: 1, Size: 3}, items)
	if resp.HasPrev {
		t.Error("first page should not have prev")
	}
	if !resp.HasNext {
		t.Error("should have next")
	}
}

func TestPaginate_LastPage(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	resp := Paginate(Request{Page: 2, Size: 3}, items)
	if !resp.HasPrev {
		t.Error("should have prev")
	}
	if resp.HasNext {
		t.Error("last page should not have next")
	}
	if len(resp.Items) != 2 {
		t.Errorf("last page len = %d, want 2", len(resp.Items))
	}
}

func TestPaginate_PageOutOfBounds(t *testing.T) {
	items := []int{1, 2, 3}
	resp := Paginate(Request{Page: 100, Size: 10}, items)
	if len(resp.Items) != 0 {
		t.Errorf("out of bounds should return empty, got %v", resp.Items)
	}
	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3", resp.Total)
	}
}

func TestPaginate_EmptySlice(t *testing.T) {
	resp := Paginate(Request{Page: 1, Size: 10}, []int{})
	if len(resp.Items) != 0 {
		t.Errorf("empty slice should return empty, got %v", resp.Items)
	}
	if resp.Total != 0 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.TotalPages != 0 {
		t.Errorf("TotalPages = %d", resp.TotalPages)
	}
}

func TestPaginate_GenericStrings(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	resp := Paginate(Request{Page: 1, Size: 2}, items)
	if len(resp.Items) != 2 || resp.Items[0] != "a" || resp.Items[1] != "b" {
		t.Errorf("items = %v", resp.Items)
	}
}

func TestPaginate_TotalPagesExactDivision(t *testing.T) {
	items := []int{1, 2, 3, 4}
	resp := Paginate(Request{Page: 1, Size: 2}, items)
	if resp.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", resp.TotalPages)
	}
}

// —— Validate ——

func TestValidate_Valid(t *testing.T) {
	r := Request{Page: 1, Size: 10, MaxSize: 100}
	if err := r.Validate(); err != nil {
		t.Errorf("valid request should pass: %v", err)
	}
}

func TestValidate_NegativePage(t *testing.T) {
	r := Request{Page: -1}
	if err := r.Validate(); err == nil {
		t.Error("negative page should fail")
	}
}

func TestValidate_SizeOverMax(t *testing.T) {
	r := Request{Size: 200, MaxSize: 100}
	if err := r.Validate(); err == nil {
		t.Error("size > max should fail")
	}
}

// —— SortBy ——

func TestSortBy_SingleField(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}
	items := []User{
		{ID: 3, Name: "c"},
		{ID: 1, Name: "a"},
		{ID: 2, Name: "b"},
	}
	fields := []SortField{{Name: "id", Desc: false}}
	SortBy(items, fields, func(a, b User, field string) bool {
		if field == "id" {
			return a.ID < b.ID
		}
		return false
	})
	if items[0].ID != 1 || items[1].ID != 2 || items[2].ID != 3 {
		t.Errorf("not sorted ascending: %+v", items)
	}
}

func TestSortBy_Descending(t *testing.T) {
	type Item struct {
		Score int
	}
	items := []Item{{Score: 3}, {Score: 1}, {Score: 2}}
	fields := []SortField{{Name: "score", Desc: true}}
	SortBy(items, fields, func(a, b Item, field string) bool {
		return a.Score < b.Score
	})
	if items[0].Score != 3 || items[1].Score != 2 || items[2].Score != 1 {
		t.Errorf("not sorted descending: %+v", items)
	}
}

func TestSortBy_MultipleFieldsPriority(t *testing.T) {
	type Emp struct {
		Dept string
		Name string
	}
	items := []Emp{
		{"A", "z"}, {"B", "a"}, {"A", "a"}, {"B", "z"},
	}
	fields := []SortField{
		{Name: "dept", Desc: false},
		{Name: "name", Desc: false},
	}
	SortBy(items, fields, func(a, b Emp, field string) bool {
		switch field {
		case "dept":
			return a.Dept < b.Dept
		case "name":
			return a.Name < b.Name
		}
		return false
	})
	// 期望：A/a, A/z, B/a, B/z
	expected := []string{"Aa", "Az", "Ba", "Bz"}
	for i, e := range items {
		combined := e.Dept + e.Name
		if combined != expected[i] {
			t.Errorf("position %d = %s, want %s", i, combined, expected[i])
		}
	}
}

func TestPaginateSorted_CombinesSortAndPage(t *testing.T) {
	type P struct {
		V int
	}
	items := []P{{3}, {1}, {4}, {1}, {5}, {9}, {2}, {6}}
	req := Request{Page: 1, Size: 3, Sort: "v:desc"}
	resp := PaginateSorted(req, items, func(a, b P, field string) bool {
		return a.V < b.V
	})
	if len(resp.Items) != 3 {
		t.Fatalf("len = %d", len(resp.Items))
	}
	if resp.Items[0].V != 9 || resp.Items[1].V != 6 || resp.Items[2].V != 5 {
		t.Errorf("not correctly sorted and paged: %+v", resp.Items)
	}
}

// —— Cursor ——

func TestCursorRequest_Normalize(t *testing.T) {
	r := CursorRequest{}.Normalize()
	if r.Size != 20 {
		t.Errorf("Size = %d", r.Size)
	}
	if r.MaxSize != 100 {
		t.Errorf("MaxSize = %d", r.MaxSize)
	}
}

// —— 综合场景 ——

func TestComposite_HTTPHandlerPattern(t *testing.T) {
	// 模拟 HTTP handler 收到分页请求后的处理流程
	allUsers := make([]int, 25)
	for i := range allUsers {
		allUsers[i] = i + 1
	}

	// 从 query string 解析（业务侧自己实现）
	req := Request{Page: 2, Size: 10}
	resp := Paginate(req, allUsers)

	if resp.Page != 2 {
		t.Errorf("Page echo = %d", resp.Page)
	}
	if resp.Total != 25 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", resp.TotalPages)
	}
	if len(resp.Items) != 10 {
		t.Errorf("Items len = %d", len(resp.Items))
	}
	if resp.Items[0] != 11 {
		t.Errorf("first item = %d, want 11", resp.Items[0])
	}
}

// —— 占位 ——

var _ = strings.Split
