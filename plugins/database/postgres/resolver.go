// URL scheme resolver 注册：postgres:// → postgres.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/database/postgres"
//	db, _ := database.NewFromURL("postgres://postgres:pass@127.0.0.1:5432/test?pool=50&lifetime=30m&sslmode=disable", tracer, meter)
//
// URL 格式（兼容 libpq URL 形式）：
//   - postgres://user:pass@host:port/dbname?pool=N&lifetime=duration&sslmode=disable
//   - postgres://host:port/dbname                  （无认证）
//   - postgres://host:port/                        （不选库）
//   - postgres://user@host:port/dbname             （仅用户名，trust 认证）
//
// query 参数：
//   - pool：MaxOpenConns（默认 0 = 不限）
//   - lifetime：ConnMaxLifetime（time.Duration 字符串，如 30m）
//   - sslmode：SSL 模式（默认 disable；可选 require/verify-ca/verify-full）
//   - connect_timeout：连接超时秒数（默认 10）
//
// postgres plugin 自带依赖（jackc/pgx/v5），所以放在 plugins 下。

package postgres

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

func init() {
	database.RegisterResolver("postgres", resolveFromURL)
}

// resolveFromURL 解析 postgres://... URL 为 database.DB 实例。
//
// URL 形态与 BuildDSN 行为对齐，但用 URL 解析替代手写拼接（更易维护）。
func resolveFromURL(rawURL string, t trace.Tracer, m metrics.Meter) (database.DB, error) {
	opts, err := parseURL(rawURL)
	if err != nil {
		return nil, err
	}
	return New(opts, t, m)
}

// parseURL 把 postgres:// URL 解析为 database.DBOptions。
//
// 单独抽出便于单元测试（避免真的调用 sql.Open 触发懒连接）。
func parseURL(rawURL string) (database.DBOptions, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return database.DBOptions{}, err
	}
	if u.Scheme != "postgres" {
		return database.DBOptions{}, fmt.Errorf("postgres: unsupported scheme %q", u.Scheme)
	}

	// SSL 模式（默认 disable）
	sslMode := defaultSSLMode
	if v := u.Query().Get("sslmode"); v != "" {
		sslMode = v
	}

	// 连接超时（默认 10s）
	connectTimeout := defaultConnectTimeoutSec
	if v := u.Query().Get("connect_timeout"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			connectTimeout = n
		}
	}

	host := u.Hostname()
	port := portOrDefault(u.Port())

	var user, pass, dbname string
	if u.User != nil {
		user = u.User.Username()
		// Password() 第二返回值 ok=false 表示未设置，pass="" 即可
		p, _ := u.User.Password()
		pass = p
	}
	// Path 形如 "/test"，去掉前导 "/"
	if u.Path != "" {
		dbname = u.Path[1:]
	}

	opts := database.DBOptions{
		Driver: driverName(),
		DSN: fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
			host, port, user, pass, dbname, sslMode, connectTimeout),
	}

	if v := u.Query().Get("pool"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			opts.MaxOpenConns = n
		}
	}
	if v := u.Query().Get("lifetime"); v != "" {
		if d, e := time.ParseDuration(v); e == nil {
			opts.ConnMaxLifetime = d
		}
	}

	return opts, nil
}
