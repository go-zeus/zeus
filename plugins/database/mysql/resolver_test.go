package mysql

import (
	"testing"
	"time"

	"github.com/go-zeus/zeus/database"
)

// TestParseURL 验证 mysql:// URL 解析为 DBOptions 的逻辑
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
			url:        "mysql://root:pass@127.0.0.1:3306/test",
			wantDSN:    "root:pass@tcp(127.0.0.1:3306)/test?" + defaultDSNParams,
			wantDriver: "mysql",
		},
		{
			name:       "with pool and lifetime",
			url:        "mysql://root:pass@127.0.0.1:3306/test?pool=50&lifetime=30m",
			wantDSN:    "root:pass@tcp(127.0.0.1:3306)/test?" + defaultDSNParams,
			wantPool:   50,
			wantLife:   30 * time.Minute,
			wantDriver: "mysql",
		},
		{
			name:       "custom port",
			url:        "mysql://user:secret@host.example:13306/dbname",
			wantDSN:    "user:secret@tcp(host.example:13306)/dbname?" + defaultDSNParams,
			wantDriver: "mysql",
		},
		{
			name:       "default port when missing",
			url:        "mysql://user:secret@host.example/dbname",
			wantDSN:    "user:secret@tcp(host.example:3306)/dbname?" + defaultDSNParams,
			wantDriver: "mysql",
		},
		{
			name:       "no auth (anonymous)",
			url:        "mysql://host.example:3306/dbname",
			wantDSN:    "tcp(host.example:3306)/dbname?" + defaultDSNParams,
			wantDriver: "mysql",
		},
		{
			name:       "no auth no port",
			url:        "mysql://host.example/dbname",
			wantDSN:    "tcp(host.example:3306)/dbname?" + defaultDSNParams,
			wantDriver: "mysql",
		},
		{
			name:       "no dbname (empty path)",
			url:        "mysql://root:pass@127.0.0.1:3306",
			wantDSN:    "root:pass@tcp(127.0.0.1:3306)/?" + defaultDSNParams,
			wantDriver: "mysql",
		},
		{
			name:       "invalid pool value ignored",
			url:        "mysql://root:pass@127.0.0.1:3306/test?pool=not-a-number",
			wantDSN:    "root:pass@tcp(127.0.0.1:3306)/test?" + defaultDSNParams,
			wantDriver: "mysql",
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

// TestResolveFromURL_Registered mysql scheme 已注册到 database
func TestResolveFromURL_Registered(t *testing.T) {
	// database.NewFromURL 应能找到 mysql resolver
	// 不实际打开连接（sql.Open 懒连接，Ping 才会真连）
	db, err := database.NewFromURL("mysql://root:pass@127.0.0.1:3306/test", nil, nil)
	if err != nil {
		t.Fatalf("NewFromURL err = %v", err)
	}
	if db == nil {
		t.Fatal("db is nil")
	}
	// 不实际使用 db（避免触发真实 MySQL 连接），仅验证类型
	// defer Close 触发 sql.Close，是安全的
	_ = db.Close()
}
