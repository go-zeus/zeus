// Package mysql 提供 MySQL 数据库的薄包装插件。
//
// 设计目的：
//   - 副作用注册 go-sql-driver/mysql 驱动（import _ 即生效）
//   - 便捷构造函数 New 复用主包 database/sql 全部 trace/metrics/tx_id 注入能力
//   - 提供 DSN 构造助手（BuildDSN）避免手写拼接易错
//
// 不做的事：
//   - 不重新实现 DB 接口（所有 hook 已在 database/sql 主包内完成）
//   - 不做 ORM / Query Builder
//   - 不做连接池监控
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/database"
//	    "github.com/go-zeus/zeus/plugins/database/mysql"
//	)
//
//	db, err := mysql.New(database.DBOptions{
//	    DSN: mysql.BuildDSN("root", "pass", "127.0.0.1", mysql.DefaultPort, "test"),
//	    MaxOpenConns: 50,
//	}, tracer, meter)
//	if err != nil { return err }
//	defer db.Close()
//
//	rows, err := db.Query(ctx, "SELECT id FROM users WHERE id = ?", 1)
package mysql

import (
	"fmt"
	"strconv"

	_ "github.com/go-sql-driver/mysql" // 副作用注册 driver 到 database/sql
	"github.com/go-zeus/zeus/database"
	sqldriver "github.com/go-zeus/zeus/database/sql"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

// DefaultPort MySQL 默认端口
const DefaultPort = 3306

// defaultDSNParams 默认 DSN 查询参数（避免常见坑）
//
//   - parseTime=true：让 TIME/DATE 自动映射到 time.Time
//   - loc=Local：使用本地时区
//   - charset=utf8mb4：完整 Unicode 支持（包括 emoji）
const defaultDSNParams = "parseTime=true&loc=Local&charset=utf8mb4"

// New 构造 MySQL-backed DB（实现 database.DB 接口）。
//
// 行为：
//   - 强制 opts.Driver = "mysql"（用户传入的值会被覆盖）
//   - 复用主包 database/sql.New，自动获得 trace span / metrics / tx_id 注入
//   - tracer/meter 为 nil 时退化为 noop（详见主包 sql.New）
//   - opts.DSN 必填，否则 sql.Open 不会报错但 Ping 时失败
//
// 用户可继续通过 database.DB 接口使用全部能力（Query/QueryRow/Exec/BeginTx/Ping/Close）。
func New(opts database.DBOptions, t trace.Tracer, m metrics.Meter) (database.DB, error) {
	// 强制使用 mysql 驱动名，避免用户拼错
	merged := opts
	merged.Driver = "mysql"
	if merged.DSN == "" {
		return nil, fmt.Errorf("mysql: DSN is required, use BuildDSN() to construct")
	}
	return sqldriver.New(merged, t, m)
}

// BuildDSN 构造标准 MySQL DSN 字符串。
//
// 格式：user:pass@tcp(host:port)/dbname?parseTime=true&loc=Local&charset=utf8mb4
//
// 行为：
//   - port == 0 时使用 DefaultPort（3306）
//   - dbname 可为空（连接到服务器但不选库）
//   - 自动追加默认 DSN 参数（parseTime / loc / charset）
//
// 高级参数（如 tls / collation / timeout）请手写完整 DSN 跳过此函数。
func BuildDSN(user, pass, host string, port int, dbname string) string {
	if port <= 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		user, pass, host, strconv.Itoa(port), dbname, defaultDSNParams)
}
