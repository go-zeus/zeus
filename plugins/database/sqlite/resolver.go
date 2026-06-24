// URL scheme resolver 注册：sqlite:// → sqlite.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/database/sqlite"
//	db, _ := database.NewFromURL("sqlite://test.db?pool=1&lifetime=5m", tracer, meter)
//
// URL 格式：
//   - sqlite://path/to/file.db                 （文件 DB，默认 ReadWriteCreate）
//   - sqlite://:memory:                        （内存 DB，每个连接独立）
//   - sqlite://:memory:?cache=shared           （共享内存 DB）
//
// query 参数：
//   - pool：MaxOpenConns（默认 0 = 不限；建议 SQLite 设为 1 避免写锁竞争）
//   - lifetime：ConnMaxLifetime（time.Duration 字符串，如 5m）
//
// sqlite plugin 自带依赖（modernc.org/sqlite），所以放在 plugins 下。

package sqlite

import (
	"net/url"
	"strconv"
	"time"

	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

func init() {
	database.RegisterResolver("sqlite", resolveFromURL)
}

// resolveFromURL 解析 sqlite://... URL 为 database.DB 实例
func resolveFromURL(rawURL string, t trace.Tracer, m metrics.Meter) (database.DB, error) {
	opts, err := parseURL(rawURL)
	if err != nil {
		return nil, err
	}
	return New(opts, t, m)
}

// parseURL 把 sqlite:// URL 解析为 database.DBOptions
//
// 单独抽出便于单元测试（避免真的调用 sql.Open 触发懒连接）
func parseURL(rawURL string) (database.DBOptions, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return database.DBOptions{}, err
	}

	opts := database.DBOptions{Driver: DriverName}

	// path 部分：去掉前导 "/"（url.Parse 会把 "//" 后当 host，把 "//path" 的 path 视为 "/path"）
	// 我们需要支持 "sqlite://:memory:" 和 "sqlite://path/to/file.db" 两种形式
	//
	// url.Parse("sqlite://:memory:") → Host=":memory:" Path=""
	// url.Parse("sqlite://test.db") → Host="test.db" Path=""
	// url.Parse("sqlite://path/to/file.db") → Host="path" Path="/to/file.db"
	path := reconstructPath(u)
	if path == "" {
		return database.DBOptions{}, errInvalidPath
	}

	// 构造 DSN
	var dsn string
	if path == ":memory:" {
		// 检查 cache=shared 参数
		if v := u.Query().Get("cache"); v == "shared" {
			dsn = BuildMemoryDSN(true)
		} else {
			dsn = BuildMemoryDSN(false)
		}
	} else {
		dsn = BuildDSN(path, defaultOpenFlag)
	}
	opts.DSN = dsn

	// pool / lifetime
	if v := u.Query().Get("pool"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
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

// reconstructPath 从 url.URL 重建 SQLite 路径
//
// 处理 url.Parse 把 sqlite://path/to/file.db 解析成 Host="path" Path="/to/file.db" 的问题
func reconstructPath(u *url.URL) string {
	if u.Path == ":memory:" {
		return ":memory:"
	}
	if u.Host == ":memory:" {
		return ":memory:"
	}
	// "sqlite://test.db" → Host="test.db" Path=""
	// "sqlite://path/to/file.db" → Host="path" Path="/to/file.db"
	if u.Host != "" && u.Path == "" {
		return u.Host
	}
	if u.Host != "" && u.Path != "" {
		return u.Host + u.Path
	}
	return u.Path
}

var errInvalidPath = &invalidPathError{}

type invalidPathError struct{}

func (e *invalidPathError) Error() string {
	return "sqlite: invalid URL path (use sqlite://path/to/file.db or sqlite://:memory:)"
}
