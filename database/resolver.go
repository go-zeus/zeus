// URL scheme resolver（与 cache.RegisterResolver / app.RegisterRegistryResolver 同构）。
//
// 设计目的：让 database 也能通过 URL 字符串切换实现，与 registry/cache 行为一致。
//
// 用户在 L1/L2/L3 配置中传入：
//
//	"mysql://user:pass@host:3306/db?pool=50&lifetime=30m"
//	"postgres://user:pass@host:5432/db?sslmode=disable"   （需 plugins/database/postgres，待补）
//	"sqlite3://file.db"                                    （待补）
//
// 与 cache.NewFromURL 的差异：database resolver 需要额外的 tracer/meter 依赖注入，
// 因此 Resolver 签名多了这两个参数。URL 不承担依赖注入职责。
//
// plugins 在 init() 中调用 database.RegisterResolver 注册自己的 scheme，
// 主包零依赖（不直接 import plugins/database/mysql 等第三方包）。

package database

import (
	"fmt"
	"strings"

	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

// Resolver 把 URL 字符串解析为 DB 实例。
//
// 参数：
//   - rawURL：连接 URL（如 "mysql://user:pass@host:3306/db"）
//   - t：trace.Tracer（每次 Query/Exec 自动埋点；nil 时由实现兜底 noop）
//   - m：metrics.Meter（每次 Query/Exec 自动上报；nil 时由实现兜底 noop）
//
// 由 plugins 在 init() 中通过 RegisterResolver 注册。
type Resolver func(rawURL string, t trace.Tracer, m metrics.Meter) (DB, error)

var resolvers = map[string]Resolver{}

// RegisterResolver 注册 URL scheme → Resolver。
//
// 由 plugins 在 init() 中调用，主包零依赖。
// 重复注册同一 scheme 会被忽略（保留首次注册者）。
func RegisterResolver(scheme string, r Resolver) {
	if scheme != "" && r != nil {
		if _, exists := resolvers[scheme]; !exists {
			resolvers[scheme] = r
		}
	}
}

// RegisteredResolvers 返回当前已注册的所有 scheme（用于诊断与文档）。
func RegisteredResolvers() map[string]struct{} {
	out := make(map[string]struct{}, len(resolvers))
	for k := range resolvers {
		out[k] = struct{}{}
	}
	return out
}

// resolveScheme 从 rawURL 提取 scheme（"mysql://host:3306" → "mysql"）。
func resolveScheme(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx > 0 {
		return rawURL[:idx]
	}
	return ""
}

// NewFromURL 按 URL scheme 构造 DB 实例。
//
// 行为：
//   - "" → 返回 (nil, nil)，调用方按需用默认实现兜底
//   - "scheme://..." → 查找 RegisterResolver 注册的解析器
//   - 未知 scheme → 返回 error
//
// tracer/meter 通过参数显式注入（URL 不承担依赖注入职责）。
// 业务方传入 nil tracer/meter 时，由实现包兜底为 noop（不报错）。
func NewFromURL(rawURL string, t trace.Tracer, m metrics.Meter) (DB, error) {
	if rawURL == "" {
		return nil, nil
	}
	scheme := resolveScheme(rawURL)
	if scheme == "" {
		return nil, fmt.Errorf("database.NewFromURL: invalid URL %q (expected scheme://...)", rawURL)
	}
	r, ok := resolvers[scheme]
	if !ok {
		return nil, fmt.Errorf("database.NewFromURL: unknown scheme %q (import the corresponding plugin, e.g. _ \"github.com/go-zeus/zeus/plugins/database/mysql\")", scheme)
	}
	return r(rawURL, t, m)
}
