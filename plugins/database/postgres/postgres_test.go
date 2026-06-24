package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-zeus/zeus/database"
)

// TestBuildDSN_DefaultPort port=0 时使用 DefaultPort
func TestBuildDSN_DefaultPort(t *testing.T) {
	got := BuildDSN("postgres", "pass", "127.0.0.1", 0, "test")
	want := "host=127.0.0.1 port=5432 user=postgres password=pass dbname=test sslmode=disable connect_timeout=10"
	if got != want {
		t.Errorf("BuildDSN() = %q, want %q", got, want)
	}
}

// TestBuildDSN_CustomPort 自定义端口生效
func TestBuildDSN_CustomPort(t *testing.T) {
	got := BuildDSN("postgres", "pass", "127.0.0.1", 15432, "test")
	want := "host=127.0.0.1 port=15432 user=postgres password=pass dbname=test sslmode=disable connect_timeout=10"
	if got != want {
		t.Errorf("BuildDSN() = %q, want %q", got, want)
	}
}

// TestBuildDSN_EmptyDBName 空数据库名合法
func TestBuildDSN_EmptyDBName(t *testing.T) {
	got := BuildDSN("postgres", "", "127.0.0.1", DefaultPort, "")
	want := "host=127.0.0.1 port=5432 user=postgres password= dbname= sslmode=disable connect_timeout=10"
	if got != want {
		t.Errorf("BuildDSN() = %q, want %q", got, want)
	}
}

// TestBuildDSNWithSSL 自定义 SSL 模式生效
func TestBuildDSNWithSSL(t *testing.T) {
	cases := []struct {
		name, sslMode, wantSSL string
	}{
		{"empty fallback to default", "", "disable"},
		{"require", "require", "require"},
		{"verify-full", "verify-full", "verify-full"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildDSNWithSSL("u", "p", "h", 0, "db", tc.sslMode)
			want := fmt.Sprintf("host=h port=5432 user=u password=p dbname=db sslmode=%s connect_timeout=10", tc.wantSSL)
			if got != want {
				t.Errorf("BuildDSNWithSSL() = %q, want %q", got, want)
			}
		})
	}
}

// TestNew_MissingDSN DSN 为空时返回错误
func TestNew_MissingDSN(t *testing.T) {
	_, err := New(database.DBOptions{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
	if err.Error() == "" {
		t.Errorf("error message should not be empty")
	}
}

// TestNew_ForcePgxDriver 强制 Driver 覆盖（即使传入错误值）
//
// 注意：sql.Open 在 Open 阶段不验证 driver 名（只在 Ping 时验证）
// 这里仅验证 New 本身不报错（Driver 不为空）
func TestNew_ForcePgxDriver(t *testing.T) {
	db, err := New(database.DBOptions{
		Driver: "wrong-driver", // 应被强制覆盖为 pgx
		DSN:    "host=127.0.0.1 port=5432 user=postgres dbname=test sslmode=disable",
	}, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()
	t.Log("New succeeded with forced driver override")
}

// —— resolver 测试 ——

// TestParseURL 验证 postgres:// URL 解析为 DBOptions 的逻辑
func TestParseURL(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		wantDSN    string
		wantPool   int
		wantLife   time.Duration
		wantDriver string
	}{
		{
			name:       "full URL with auth",
			url:        "postgres://postgres:pass@127.0.0.1:5432/test",
			wantDSN:    "host=127.0.0.1 port=5432 user=postgres password=pass dbname=test sslmode=disable connect_timeout=10",
			wantDriver: "pgx",
		},
		{
			name:       "with pool and lifetime",
			url:        "postgres://postgres:pass@127.0.0.1:5432/test?pool=50&lifetime=30m",
			wantDSN:    "host=127.0.0.1 port=5432 user=postgres password=pass dbname=test sslmode=disable connect_timeout=10",
			wantPool:   50,
			wantLife:   30 * time.Minute,
			wantDriver: "pgx",
		},
		{
			name:       "custom port",
			url:        "postgres://user:secret@host.example:15432/dbname",
			wantDSN:    "host=host.example port=15432 user=user password=secret dbname=dbname sslmode=disable connect_timeout=10",
			wantDriver: "pgx",
		},
		{
			name:       "default port when missing",
			url:        "postgres://user:secret@host.example/dbname",
			wantDSN:    "host=host.example port=5432 user=user password=secret dbname=dbname sslmode=disable connect_timeout=10",
			wantDriver: "pgx",
		},
		{
			name:       "sslmode require",
			url:        "postgres://postgres:pass@127.0.0.1:5432/test?sslmode=require",
			wantDSN:    "host=127.0.0.1 port=5432 user=postgres password=pass dbname=test sslmode=require connect_timeout=10",
			wantDriver: "pgx",
		},
		{
			name:       "connect_timeout override",
			url:        "postgres://postgres:pass@127.0.0.1:5432/test?connect_timeout=30",
			wantDSN:    "host=127.0.0.1 port=5432 user=postgres password=pass dbname=test sslmode=disable connect_timeout=30",
			wantDriver: "pgx",
		},
		{
			name:       "no auth (anonymous)",
			url:        "postgres://127.0.0.1:5432/test",
			wantDSN:    "host=127.0.0.1 port=5432 user= password= dbname=test sslmode=disable connect_timeout=10",
			wantDriver: "pgx",
		},
		{
			name:       "no dbname (empty path)",
			url:        "postgres://postgres:pass@127.0.0.1:5432",
			wantDSN:    "host=127.0.0.1 port=5432 user=postgres password=pass dbname= sslmode=disable connect_timeout=10",
			wantDriver: "pgx",
		},
		{
			name:       "invalid pool value ignored",
			url:        "postgres://postgres:pass@127.0.0.1:5432/test?pool=not-a-number",
			wantDSN:    "host=127.0.0.1 port=5432 user=postgres password=pass dbname=test sslmode=disable connect_timeout=10",
			wantDriver: "pgx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseURL(tc.url)
			if err != nil {
				t.Fatalf("parseURL err = %v", err)
			}
			if opts.DSN != tc.wantDSN {
				t.Errorf("DSN = %q, want %q", opts.DSN, tc.wantDSN)
			}
			if opts.Driver != tc.wantDriver {
				t.Errorf("Driver = %q, want %q", opts.Driver, tc.wantDriver)
			}
			if opts.MaxOpenConns != tc.wantPool {
				t.Errorf("MaxOpenConns = %d, want %d", opts.MaxOpenConns, tc.wantPool)
			}
			if opts.ConnMaxLifetime != tc.wantLife {
				t.Errorf("ConnMaxLifetime = %v, want %v", opts.ConnMaxLifetime, tc.wantLife)
			}
		})
	}
}

// TestResolveFromURL_Registered postgres scheme 已注册到 database
func TestResolveFromURL_Registered(t *testing.T) {
	// 不实际打开连接（sql.Open 懒连接，Ping 才会真连）
	db, err := database.NewFromURL("postgres://postgres:pass@127.0.0.1:5432/test", nil, nil)
	if err != nil {
		t.Fatalf("NewFromURL err = %v", err)
	}
	if db == nil {
		t.Fatal("db is nil")
	}
	_ = db.Close()
}
