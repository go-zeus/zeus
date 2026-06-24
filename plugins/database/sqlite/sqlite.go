// Package sqlite 提供 SQLite 数据库的薄包装插件。
//
// 设计目的：
//   - 副作用注册 modernc.org/sqlite 驱动（纯 Go，无 cgo，跨平台编译零障碍）
//   - 便捷构造函数 New 复用主包 database/sql 全部 trace/metrics/tx_id 注入能力
//   - 适配本地开发/测试/嵌入式场景（vs MySQL/Postgres 用于生产）
//
// 不做的事：
//   - 不重新实现 DB 接口（所有 hook 已在 database/sql 主包内完成）
//   - 不做 ORM / Query Builder
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/database"
//	    "github.com/go-zeus/zeus/plugins/database/sqlite"
//	)
//
//	db, err := sqlite.New(database.DBOptions{
//	    DSN: sqlite.BuildDSN("test.db", sqlite.OpenReadWriteCreate),
//	    MaxOpenConns: 1, // SQLite 写并发建议 1（避免锁竞争）
//	}, tracer, meter)
//	if err != nil { return err }
//	defer db.Close()
//
//	_, err = db.Exec(ctx, "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)")
package sqlite

import (
	"fmt"

	_ "modernc.org/sqlite" // 副作用注册 driver 到 database/sql

	"github.com/go-zeus/zeus/database"
	sqldriver "github.com/go-zeus/zeus/database/sql"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

// 默认 driver 名（modernc.org/sqlite 用 "sqlite"，注意没有 "3"）
//
// modernc.org/sqlite 是纯 Go 实现（无 cgo），跨平台编译零障碍；
// 如果业务侧必须用 mattn/go-sqlite3（cgo），自行 import + 把 opts.Driver 改为 "sqlite3"。
const DriverName = "sqlite"

// OpenFlag 文件打开模式
type OpenFlag int

const (
	// OpenReadOnly 只读（查询场景，文件已存在）
	OpenReadOnly OpenFlag = 1
	// OpenReadWrite 读写（文件已存在）
	OpenReadWrite OpenFlag = 2
	// OpenReadWriteCreate 读写 + 不存在则创建（默认，本地开发场景）
	OpenReadWriteCreate OpenFlag = 6
)

// defaultOpenFlag 默认打开模式（读写 + 创建）
const defaultOpenFlag = OpenReadWriteCreate

// New 构造 SQLite-backed DB（实现 database.DB 接口）
//
// 行为：
//   - 强制 opts.Driver = "sqlite"（用户传入的值会被覆盖）
//   - 复用主包 database/sql.New，自动获得 trace span / metrics / tx_id 注入
//   - tracer/meter 为 nil 时退化为 noop（详见主包 sql.New）
//   - opts.DSN 必填
//
// 性能建议：
//   - 单机 SQLite 写并发建议 MaxOpenConns=1（避免 SQLITE_BUSY 锁竞争）
//   - 读并发无限制（WAL 模式下读写也不互斥）
//
// 用户可继续通过 database.DB 接口使用全部能力（Query/QueryRow/Exec/BeginTx/Ping/Close）。
func New(opts database.DBOptions, t trace.Tracer, m metrics.Meter) (database.DB, error) {
	merged := opts
	merged.Driver = DriverName
	if merged.DSN == "" {
		return nil, fmt.Errorf("sqlite: DSN is required, use BuildDSN() to construct")
	}
	return sqldriver.New(merged, t, m)
}

// BuildDSN 构造 SQLite DSN 字符串
//
// 格式：file:path/to/db?param1=value1&param2=value2
//
// 行为：
//   - path 为空时返回错误
//   - 自动追加默认参数：
//     _pragma=busy_timeout(5000)  等 5s 而非立刻 SQLITE_BUSY
//     _pragma=foreign_keys(1)     启用外键约束（默认关闭）
//     _pragma=journal_mode(WAL)   WAL 模式，提升读并发
func BuildDSN(path string, flag OpenFlag) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
}

// BuildMemoryDSN 构造内存数据库 DSN（用于单测 / 临时缓存）
//
// 格式：file::memory:?cache=shared&_pragma=journal_mode(MEMORY)
//
// cache=shared：同进程多个 *sql.DB 句柄共享同一份内存数据库
// 不开 cache=shared：每个 *sql.DB 独立内存数据库（隔离测试场景）
func BuildMemoryDSN(shared bool) string {
	if shared {
		return "file::memory:?cache=shared&_pragma=journal_mode(MEMORY)"
	}
	return "file::memory:?_pragma=journal_mode(MEMORY)"
}
