package sqlite

import (
	"testing"
)

// TestParseURL_FilePath 文件路径 URL
func TestParseURL_FilePath(t *testing.T) {
	opts, err := parseURL("sqlite://test.db")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.Driver != DriverName {
		t.Errorf("Driver = %q, want %q", opts.Driver, DriverName)
	}
	if opts.DSN == "" {
		t.Error("DSN should not be empty")
	}
	if opts.DSN != "file:test.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)" {
		t.Errorf("DSN = %q, want file:test.db?...", opts.DSN)
	}
}

// TestParseURL_NestedPath 嵌套路径
func TestParseURL_NestedPath(t *testing.T) {
	opts, err := parseURL("sqlite://path/to/data.db")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.DSN == "" {
		t.Error("DSN should not be empty")
	}
	// 期望 path/to/data.db
	if opts.DSN != "file:path/to/data.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)" {
		t.Errorf("DSN = %q", opts.DSN)
	}
}

// TestParseURL_MemoryBasic 内存 DB 基础形式
func TestParseURL_MemoryBasic(t *testing.T) {
	opts, err := parseURL("sqlite://:memory:")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.DSN != "file::memory:?_pragma=journal_mode(MEMORY)" {
		t.Errorf("DSN = %q, want non-shared memory", opts.DSN)
	}
}

// TestParseURL_MemoryShared 共享内存 DB
func TestParseURL_MemoryShared(t *testing.T) {
	opts, err := parseURL("sqlite://:memory:?cache=shared")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.DSN != "file::memory:?cache=shared&_pragma=journal_mode(MEMORY)" {
		t.Errorf("DSN = %q, want shared memory", opts.DSN)
	}
}

// TestParseURL_PoolAndLifetime query 参数 pool / lifetime
func TestParseURL_PoolAndLifetime(t *testing.T) {
	opts, err := parseURL("sqlite://test.db?pool=1&lifetime=5m")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.MaxOpenConns != 1 {
		t.Errorf("MaxOpenConns = %d, want 1", opts.MaxOpenConns)
	}
	if opts.ConnMaxLifetime.Minutes() != 5 {
		t.Errorf("ConnMaxLifetime = %v, want 5m", opts.ConnMaxLifetime)
	}
}

// TestParseURL_InvalidPool 非法 pool 值静默忽略
func TestParseURL_InvalidPool(t *testing.T) {
	opts, err := parseURL("sqlite://test.db?pool=notanumber")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.MaxOpenConns != 0 {
		t.Errorf("MaxOpenConns = %d, want 0 (invalid value ignored)", opts.MaxOpenConns)
	}
}

// TestParseURL_InvalidLifetime 非法 lifetime 静默忽略
func TestParseURL_InvalidLifetime(t *testing.T) {
	opts, err := parseURL("sqlite://test.db?lifetime=notaduration")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	if opts.ConnMaxLifetime != 0 {
		t.Errorf("ConnMaxLifetime = %v, want 0 (invalid value ignored)", opts.ConnMaxLifetime)
	}
}

// TestParseURL_UnknownQuery 未知 query 参数静默忽略
func TestParseURL_UnknownQuery(t *testing.T) {
	opts, err := parseURL("sqlite://test.db?unknown=value&foo=bar")
	if err != nil {
		t.Fatalf("parseURL err: %v", err)
	}
	// 不会 panic，DSN 正常
	if opts.DSN == "" {
		t.Error("DSN should not be empty")
	}
}

// TestBuildDSN_WALPragmas DSN 包含默认 pragma
func TestBuildDSN_WALPragmas(t *testing.T) {
	dsn := BuildDSN("test.db", OpenReadWriteCreate)
	if dsn == "" {
		t.Error("BuildDSN returned empty for valid path")
	}
	for _, want := range []string{"file:test.db", "busy_timeout(5000)", "foreign_keys(1)", "journal_mode(WAL)"} {
		if !contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}

// TestBuildDSN_EmptyPath 空路径返回空 DSN
func TestBuildDSN_EmptyPath(t *testing.T) {
	if dsn := BuildDSN("", OpenReadWriteCreate); dsn != "" {
		t.Errorf("BuildDSN(empty) = %q, want empty", dsn)
	}
}

// TestBuildMemoryDSN 内存 DSN 包含 journal_mode(MEMORY)
func TestBuildMemoryDSN(t *testing.T) {
	cases := []struct {
		shared bool
		want   string
	}{
		{false, "file::memory:?_pragma=journal_mode(MEMORY)"},
		{true, "file::memory:?cache=shared&_pragma=journal_mode(MEMORY)"},
	}
	for _, tc := range cases {
		got := BuildMemoryDSN(tc.shared)
		if got != tc.want {
			t.Errorf("BuildMemoryDSN(%v) = %q, want %q", tc.shared, got, tc.want)
		}
	}
}

// TestDefaultConstants 默认常量合理
func TestDefaultConstants(t *testing.T) {
	if DriverName != "sqlite" {
		t.Errorf("DriverName = %q, want sqlite", DriverName)
	}
	if defaultOpenFlag != OpenReadWriteCreate {
		t.Errorf("defaultOpenFlag = %v, want OpenReadWriteCreate", defaultOpenFlag)
	}
}

// TestOpenFlags 标志位语义检查
func TestOpenFlags(t *testing.T) {
	// OpenReadWriteCreate = 6（=OpenReadWrite | 4）
	if OpenReadWriteCreate != 6 {
		t.Errorf("OpenReadWriteCreate = %d, want 6", OpenReadWriteCreate)
	}
	if OpenReadOnly != 1 {
		t.Errorf("OpenReadOnly = %d, want 1", OpenReadOnly)
	}
	if OpenReadWrite != 2 {
		t.Errorf("OpenReadWrite = %d, want 2", OpenReadWrite)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
