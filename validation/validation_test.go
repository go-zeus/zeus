package validation

import (
	stderrors "errors"
	"strings"
	"testing"

	zeuserrors "github.com/go-zeus/zeus/errors"
)

// —— Required ——

func TestRequired_NilFails(t *testing.T) {
	v := New().Required("id", nil)
	if !v.HasErrors() {
		t.Error("nil should fail Required")
	}
}

func TestRequired_EmptyStringFails(t *testing.T) {
	if !New().Required("name", "").HasErrors() {
		t.Error("empty string should fail Required")
	}
}

func TestRequired_EmptySliceFails(t *testing.T) {
	if !New().Required("tags", []string{}).HasErrors() {
		t.Error("empty slice should fail Required")
	}
	if !New().Required("tags", []int{}).HasErrors() {
		t.Error("empty int slice should fail Required")
	}
}

func TestRequired_EmptyMapFails(t *testing.T) {
	if !New().Required("meta", map[string]any{}).HasErrors() {
		t.Error("empty map should fail Required")
	}
}

func TestRequired_NonEmptyPasses(t *testing.T) {
	v := New().
		Required("a", "x").
		Required("b", 1).
		Required("c", []int{1}).
		Required("d", map[string]int{"k": 1})
	if v.HasErrors() {
		t.Errorf("non-empty values should pass, got %d errors", v.Count())
	}
}

func TestRequired_NilPtrFails(t *testing.T) {
	var p *int
	if !New().Required("ptr", p).HasErrors() {
		t.Error("nil ptr should fail Required")
	}
}

// —— MinLen / MaxLen ——

func TestMinLen(t *testing.T) {
	cases := []struct {
		val    any
		min    int
		expect bool
	}{
		{"abc", 3, true},
		{"abc", 4, false},
		{[]int{1, 2, 3}, 3, true},
		{[]int{1, 2, 3}, 4, false},
		{"", 1, false},
	}
	for _, c := range cases {
		v := New().MinLen("f", c.val, c.min)
		if v.HasErrors() == c.expect {
			t.Errorf("MinLen(%v,%d): expect pass=%v, got errs=%v", c.val, c.min, c.expect, v.HasErrors())
		}
	}
}

func TestMaxLen(t *testing.T) {
	if !New().MaxLen("f", "abcd", 3).HasErrors() {
		t.Error("len > max should fail")
	}
	if New().MaxLen("f", "abc", 3).HasErrors() {
		t.Error("len == max should pass")
	}
}

func TestRange_Length(t *testing.T) {
	v := New().Range("f", "ab", 3, 5)
	if !v.HasErrors() {
		t.Error("len=2 outside [3,5] should fail")
	}
}

// —— Min / Max 数值 ——

func TestMin_Int(t *testing.T) {
	if !New().Min("age", -1, 0).HasErrors() {
		t.Error("-1 < 0 should fail")
	}
	if New().Min("age", 0, 0).HasErrors() {
		t.Error("0 >= 0 should pass")
	}
}

func TestMax_Float(t *testing.T) {
	if !New().Max("ratio", 1.5, 1.0).HasErrors() {
		t.Error("1.5 > 1.0 should fail")
	}
}

func TestMin_NonNumeric(t *testing.T) {
	v := New().Min("f", "not-a-num", 0)
	if !v.HasErrors() {
		t.Error("string passed to Min should fail with 'not numeric'")
	}
}

// —— In ——

func TestIn(t *testing.T) {
	v := New().In("role", "admin", "user", "guest", "admin")
	if v.HasErrors() {
		t.Error("'admin' in candidates should pass")
	}
	v = New().In("role", "hacker", "user", "guest")
	if !v.HasErrors() {
		t.Error("'hacker' not in candidates should fail")
	}
}

// —— Email / URL / IP ——

func TestEmail(t *testing.T) {
	v := New().Email("e", "a@b.com")
	if v.HasErrors() {
		t.Error("'a@b.com' should be valid email")
	}
	v = New().Email("e", "not-email")
	if !v.HasErrors() {
		t.Error("'not-email' should fail email validation")
	}
	// 空字符串跳过（交给 Required）
	v = New().Email("e", "")
	if v.HasErrors() {
		t.Error("empty string should be skipped")
	}
}

func TestURL(t *testing.T) {
	if New().URL("u", "https://example.com").HasErrors() {
		t.Error("valid URL failed")
	}
	if !New().URL("u", "not a url").HasErrors() {
		t.Error("invalid URL should fail")
	}
}

func TestIP(t *testing.T) {
	if New().IP("i", "127.0.0.1").HasErrors() {
		t.Error("127.0.0.1 should be valid IP")
	}
	if !New().IP("i", "not-ip").HasErrors() {
		t.Error("'not-ip' should fail")
	}
}

func TestIPv4(t *testing.T) {
	if New().IPv4("i", "192.168.1.1").HasErrors() {
		t.Error("192.168.1.1 should be valid IPv4")
	}
	if !New().IPv4("i", "::1").HasErrors() {
		t.Error("::1 is IPv6 not IPv4")
	}
}

func TestIPv6(t *testing.T) {
	if New().IPv6("i", "::1").HasErrors() {
		t.Error("::1 should be valid IPv6")
	}
	if !New().IPv6("i", "127.0.0.1").HasErrors() {
		t.Error("127.0.0.1 should fail IPv6")
	}
}

// —— 字符串格式 ——

func TestMatch(t *testing.T) {
	if New().Match("code", "ABC123", "^[A-Z]+[0-9]+$").HasErrors() {
		t.Error("pattern should match")
	}
	if !New().Match("code", "abc", "^[A-Z]+$").HasErrors() {
		t.Error("pattern should not match")
	}
}

func TestAlpha(t *testing.T) {
	if New().Alpha("n", "Hello").HasErrors() {
		t.Error("'Hello' is alpha")
	}
	if !New().Alpha("n", "Hello123").HasErrors() {
		t.Error("'Hello123' is not pure alpha")
	}
}

func TestAlphanumeric(t *testing.T) {
	if New().Alphanumeric("n", "abc123").HasErrors() {
		t.Error("'abc123' is alphanumeric")
	}
	if !New().Alphanumeric("n", "abc-123").HasErrors() {
		t.Error("'abc-123' contains dash")
	}
}

func TestNumeric(t *testing.T) {
	if New().Numeric("phone", "13800138000").HasErrors() {
		t.Error("phone should be numeric")
	}
	if !New().Numeric("phone", "13800-138000").HasErrors() {
		t.Error("dash should fail numeric")
	}
}

func TestNoWhitespace(t *testing.T) {
	if !New().NoWhitespace("user", "ab cd").HasErrors() {
		t.Error("'ab cd' contains space")
	}
}

// —— Rule 自定义 ——

func TestRule_Custom(t *testing.T) {
	v := New()
	Rule(v, "username", "admin", func(s string) bool {
		return s != "admin"
	}, "must not be reserved")
	if !v.HasErrors() {
		t.Error("'admin' should fail custom rule")
	}
}

// —— Add 直接追加 ——

func TestAdd(t *testing.T) {
	v := New().Add("custom", "is wrong")
	if v.Count() != 1 {
		t.Errorf("Count = %d, want 1", v.Count())
	}
}

// —— Err 合并 ——

func TestErr_NoErrors(t *testing.T) {
	if New().Err() != nil {
		t.Error("no errors should return nil")
	}
}

func TestErr_SingleError(t *testing.T) {
	v := New().Required("x", "")
	err := v.Err()
	if err == nil {
		t.Fatal("expected error")
	}
	e, ok := err.(*zeuserrors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if e.Code != 400 {
		t.Errorf("Code = %d, want 400", e.Code)
	}
	if e.Reason != "VALIDATION_FAILED" {
		t.Errorf("Reason = %q", e.Reason)
	}
}

func TestErr_MultipleErrorsMerged(t *testing.T) {
	v := New().
		Required("a", "").
		Required("b", "").
		Required("c", "")
	err := v.Err()
	if err == nil {
		t.Fatal("expected error")
	}
	e, ok := err.(*zeuserrors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	causes, ok := e.Metadata["causes"].([]*zeuserrors.Error)
	if !ok {
		t.Fatalf("causes not found or wrong type: %T", e.Metadata["causes"])
	}
	if len(causes) != 3 {
		t.Errorf("causes count = %d, want 3", len(causes))
	}
}

func TestErr_IsValidationFailed(t *testing.T) {
	v := New().Required("x", "")
	err := v.Err()
	if !stderrors.Is(err, ErrValidationFailed) {
		t.Error("err should match ErrValidationFailed via Is")
	}
}

// —— 综合场景 ——

func TestComposite_FormPattern(t *testing.T) {
	// 模拟注册表单校验
	type Form struct {
		Username string
		Email    string
		Age      int
		Password string
	}
	form := Form{
		Username: "ab",        // 太短
		Email:    "not-email", // 格式错
		Age:      -1,          // 负数
		Password: "short",     // 太短
	}

	v := New().
		MinLen("username", form.Username, 3).
		MaxLen("username", form.Username, 20).
		Email("email", form.Email).
		Min("age", form.Age, 0).
		Max("age", form.Age, 150).
		MinLen("password", form.Password, 8)

	err := v.Err()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	if v.Count() != 4 {
		t.Errorf("expected 4 errors, got %d", v.Count())
	}

	// 错误信息应包含字段名
	s := err.Error()
	for _, field := range []string{"username", "email", "age", "password"} {
		if !strings.Contains(s, field) {
			t.Errorf("error message should mention %q: %s", field, s)
		}
	}
}

// —— 占位 ——
