// Package postgres 提供 PostgreSQL 数据库的薄包装插件。
//
// 设计目的：
//   - 副作用注册 jackc/pgx/v5/stdlib 驱动（import _ 即生效）
//   - 便捷构造函数 New 复用主包 database/sql 全部 trace/metrics/tx_id 注入能力
//   - 提供 DSN 构造助手（BuildDSN）避免手写拼接易错
//
// 不做的事：
//   - 不重新实现 DB 接口（所有 hook 已在 database/sql 主包内完成）
//   - 不做 ORM / Query Builder
//   - 不做连接池监控
//
// 选型说明：
//   - 用 pgx/v5 而非 lib/pq：lib/pq 已 archived（仅维护），pgx 是活跃维护的 modern driver
//   - 用 pgx 的 stdlib adapter（而非 native API）：保留 stdlib database/sql 接口兼容
//   - driver 名注册为 "pgx"（stdlib adapter 默认行为），可通过 SQL_DRIVER=pgx 跨工具复用
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/database"
//	    "github.com/go-zeus/zeus/plugins/database/postgres"
//	)
//
//	db, err := postgres.New(database.DBOptions{
//	    DSN: postgres.BuildDSN("postgres", "pass", "127.0.0.1", postgres.DefaultPort, "test"),
//	    MaxOpenConns: 50,
//	}, tracer, meter)
//	if err != nil { return err }
//	defer db.Close()
//
//	rows, err := db.Query(ctx, "SELECT id FROM users WHERE id = $1", 1) // PG 用 $1/$2 占位符
package postgres

import (
	"fmt"
	"strconv"

	"github.com/go-zeus/zeus/database"
	sqldriver "github.com/go-zeus/zeus/database/sql"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
	_ "github.com/jackc/pgx/v5/stdlib" // 副作用注册 driver "pgx" 到 database/sql
)

// DefaultPort PostgreSQL 默认端口
const DefaultPort = 5432

// defaultSSLMode 默认 SSL 模式
//
//   - disable：不加密（开发/内网默认）
//   - require：强制 SSL（生产建议）
//   - verify-full：验证证书（金融级）
//
// 默认 disable 与 PG 客户端 libpq 默认 prefer 不同，原因是 Zeus 主要在容器内网部署，
// disable 避免自签证书的常见坑；用户可通过 BuildDSNWithSSL 或手写 DSN 覆盖。
const defaultSSLMode = "disable"

// defaultConnectTimeoutSec 默认连接超时（秒）
//
// PG 的 connect_timeout 参数，避免无密码时挂死。
const defaultConnectTimeoutSec = 10

// New 构造 PostgreSQL-backed DB（实现 database.DB 接口）。
//
// 行为：
//   - 强制 opts.Driver = "pgx"（用户传入的值会被覆盖）
//   - 复用主包 database/sql.New，自动获得 trace span / metrics / tx_id 注入
//   - tracer/meter 为 nil 时退化为 noop（详见主包 sql.New）
//   - opts.DSN 必填，否则 sql.Open 不会报错但 Ping 时失败
//
// 用户可继续通过 database.DB 接口使用全部能力（Query/QueryRow/Exec/BeginTx/Ping/Close）。
//
// 占位符注意：PG 用 $1/$2/$3 而非 ?，迁移代码时需改写。
func New(opts database.DBOptions, t trace.Tracer, m metrics.Meter) (database.DB, error) {
	merged := opts
	merged.Driver = "pgx"
	if merged.DSN == "" {
		return nil, fmt.Errorf("postgres: DSN is required, use BuildDSN() to construct")
	}
	return sqldriver.New(merged, t, m)
}

// BuildDSN 构造标准 libpq 关键字格式 DSN。
//
// 格式：host=H port=P user=U password=P dbname=D sslmode=disable connect_timeout=10
//
// 行为：
//   - port == 0 时使用 DefaultPort（5432）
//   - dbname 可为空（连接到服务器但不选库）
//   - password 可为空（trust/ident 认证场景）
//   - 自动追加 sslmode=disable + connect_timeout=10（避免常见坑）
//
// 高级参数（如 sslrootcert / application_name / search_path）请手写 DSN 或用 BuildDSNWithSSL。
func BuildDSN(user, pass, host string, port int, dbname string) string {
	if port <= 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		host, port, user, pass, dbname, defaultSSLMode, defaultConnectTimeoutSec)
}

// BuildDSNWithSSL 构造带自定义 SSL 模式的 DSN。
//
// sslMode 取值：
//   - "" → 使用默认 disable
//   - disable / require / verify-ca / verify-full → 透传给 libpq
//
// 示例：
//
//	postgres.BuildDSNWithSSL("u", "p", "db.example", 5432, "app", "require")
func BuildDSNWithSSL(user, pass, host string, port int, dbname, sslMode string) string {
	if sslMode == "" {
		sslMode = defaultSSLMode
	}
	if port <= 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		host, port, user, pass, dbname, sslMode, defaultConnectTimeoutSec)
}

// driverName 返回驱动名（便于 resolver_test 复用而不暴露常量到文档）
func driverName() string { return "pgx" }

// portOrDefault 内部复用：返回字符串端口号，便于 resolver 拼接
func portOrDefault(port string) string {
	if port == "" {
		return strconv.Itoa(DefaultPort)
	}
	return port
}
