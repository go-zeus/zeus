package uuid

import (
	"crypto/rand"
	"reflect"
	"regexp"
	"testing"
)

// uuidPattern 预编译正则，避免在循环中反复编译（SA6000）
var uuidPattern = regexp.MustCompile(
	`[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}`,
)

func TestGenerateUUID(t *testing.T) {
	prev, err := GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		id, err := GenerateUUID()
		if err != nil {
			t.Fatal(err)
		}
		if prev == id {
			t.Fatalf("Should get a new ID!")
		}

		if !uuidPattern.MatchString(id) {
			t.Fatalf("expected match %s", id)
		}
	}
}

func TestParseUUID(t *testing.T) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("failed to read random bytes: %v", err)
	}

	uuidStr, err := FormatUUID(buf)
	if err != nil {
		t.Fatal(err)
	}

	parsedStr, err := ParseUUID(uuidStr)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(parsedStr, buf) {
		t.Fatalf("mismatched buffers")
	}
}

func BenchmarkGenerateUUID(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, _ = GenerateUUID()
	}
}

// TestUUID 验证 New() 生成非空且格式正确的 UUID
func TestUUID(t *testing.T) {
	got := New()
	if got == "" {
		t.Error("New() 返回空字符串")
	}
	matched, err := regexp.MatchString(
		"[\\da-f]{8}-[\\da-f]{4}-[\\da-f]{4}-[\\da-f]{4}-[\\da-f]{12}", got)
	if !matched || err != nil {
		t.Errorf("New() 返回值 %q 格式不正确, matched=%v, err=%v", got, matched, err)
	}
}

// TestUUID_Uniqueness 验证多次生成的 UUID 互不相同
func TestUUID_Uniqueness(t *testing.T) {
	const n = 100
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := New()
		if _, ok := seen[id]; ok {
			t.Errorf("UUID 重复: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// TestUUID_Length 验证 UUID 长度为 36
func TestUUID_Length(t *testing.T) {
	got := New()
	if len(got) != 36 {
		t.Errorf("New() 长度 = %d, 期望 36 (值: %q)", len(got), got)
	}
}

// TestGenerateUUID_V4Markers 验证 RFC 4122 v4 标记位正确
//
// v4 UUID 第 14 位（第 3 段首位）应为 '4'，第 17 位（第 4 段首位）应为 '8' 或 '9' 或 'a' 或 'b'
// 这两个标记确保生成的 UUID 能通过 PostgreSQL uuid 列、Python uuid.UUID 等严格校验
func TestGenerateUUID_V4Markers(t *testing.T) {
	const n = 1000
	for i := 0; i < n; i++ {
		id := New()

		// 第 3 段首位 = '4'（version 4）
		// UUID 格式：xxxxxxxx-xxxx-Mxxx-Nxxx-xxxxxxxxxxxx
		//                       ^14   ^19
		if id[14] != '4' {
			t.Errorf("UUID %q: version nibble = %c, want '4' (version 4)", id, id[14])
		}

		// 第 4 段首位 ∈ {'8','9','a','b'}（variant 1）
		switch id[19] {
		case '8', '9', 'a', 'b':
			// OK
		default:
			t.Errorf("UUID %q: variant nibble = %c, want one of 8/9/a/b", id, id[19])
		}
	}
}

// TestIsV4 用 ParseUUID 后的字节验证 IsV4 函数
func TestIsV4(t *testing.T) {
	// GenerateUUID 生成的应是 v4
	id := New()
	buf, err := ParseUUID(id)
	if err != nil {
		t.Fatalf("ParseUUID err: %v", err)
	}
	if !IsV4(buf) {
		t.Error("GenerateUUID result should pass IsV4 check")
	}

	// 手动构造一个非 v4 的 bytes（version=0，variant=0）
	nonV4 := make([]byte, 16)
	// 默认全 0：version nibble = 0，variant bits = 00 → 非 v4
	if IsV4(nonV4) {
		t.Error("all-zero bytes should not pass IsV4")
	}

	// 长度不对返回 false
	if IsV4(make([]byte, 8)) {
		t.Error("short bytes should not pass IsV4")
	}
	if IsV4(nil) {
		t.Error("nil should not pass IsV4")
	}
}

// TestGenerateUUID_PostgreSQLCompatible 验证生成的 UUID 可被 PostgreSQL uuid 类型接受
//
// PostgreSQL 校验规则（src/backend/utils/adt/uuid.c）：
//  1. 36 字符，第 9/14/19/24 是 '-'
//  2. 其余字符是 hex
//
// 注：PostgreSQL 实际不强制 v4 标记，但许多业务代码会校验。
// 本测试通过完整正则 + v4 标记位联合验证
func TestGenerateUUID_PostgreSQLCompatible(t *testing.T) {
	// 标准 UUID v4 正则（含版本和变体标记）
	pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	r := regexp.MustCompile(pattern)

	for i := 0; i < 1000; i++ {
		id := New()
		if !r.MatchString(id) {
			t.Errorf("UUID %q does not match strict v4 pattern", id)
		}
	}
}
