// URL scheme resolver 注册：mysql:// → mysql.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/database/mysql"
//	db, _ := database.NewFromURL("mysql://root:pass@127.0.0.1:3306/test?pool=50&lifetime=30m", tracer, meter)
//
// URL 格式：
//   - mysql://user:pass@host:port/dbname?pool=N&lifetime=duration
//   - mysql://host:port/dbname                  （无认证）
//   - mysql://host:port/                        （不选库）
//
// query 参数：
//   - pool：MaxOpenConns（默认 0 = 不限）
//   - lifetime：ConnMaxLifetime（time.Duration 字符串，如 30m）
//
// mysql plugin 自带依赖（go-sql-driver/mysql），所以放在 plugins 下。

package mysql

import (
	"net/url"
	"strconv"
	"time"

	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

func init() {
	database.RegisterResolver("mysql", resolveFromURL)
}

// resolveFromURL 解析 mysql://... URL 为 database.DB 实例。
//
// URL 形态与 BuildDSN 行为对齐，但用 URL 解析替代手写拼接（更易维护）。
func resolveFromURL(rawURL string, t trace.Tracer, m metrics.Meter) (database.DB, error) {
	opts, err := parseURL(rawURL)
	if err != nil {
		return nil, err
	}
	return New(opts, t, m)
}

// parseURL 把 mysql:// URL 解析为 database.DBOptions。
//
// 单独抽出便于单元测试（避免真的调用 sql.Open 触发懒连接）。
func parseURL(rawURL string) (database.DBOptions, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return database.DBOptions{}, err
	}

	opts := database.DBOptions{Driver: "mysql"}
	if u.User != nil {
		user := u.User.Username()
		pass, _ := u.User.Password()
		host := u.Hostname()
		port, _ := strconv.Atoi(u.Port())
		dbname := u.Path
		if dbname != "" {
			dbname = dbname[1:] // 去掉前导 "/"
		}
		opts.DSN = BuildDSN(user, pass, host, port, dbname)
	} else {
		// 无认证：手写 DSN（BuildDSN 假设有 user/pass）
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			port = strconv.Itoa(DefaultPort)
		}
		dbname := u.Path
		if dbname != "" {
			dbname = dbname[1:]
		}
		opts.DSN = "tcp(" + host + ":" + port + ")/" + dbname + "?" + defaultDSNParams
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
