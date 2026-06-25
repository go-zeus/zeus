// URL scheme resolver（与 app.RegisterRegistryResolver 同构）。
//
// 设计目的：让 cache 也能通过 URL 字符串切换实现，与 registry 行为一致。
//
// 用户在 L1/L2/L3 配置中传入：
//
//	"memory://?cleanup=60s"      → cache/memory
//	"redis://127.0.0.1:6379/0"   → plugins/cache/redis（需 import _ 该包）
//	"memcached://127.0.0.1:11211" → plugins/cache/memcached（待补）
//
// plugins 在 init() 中调用 cache.RegisterResolver 注册自己的 scheme，
// 主包零依赖（不直接 import plugins/cache/redis 等第三方包）。

package cache

import (
	"fmt"
	"strings"
)

// Resolver 把 URL 字符串解析为 Cache 实例。
//
// 由 plugins 在 init() 中通过 RegisterResolver 注册。
// 例如 plugins/cache/redis:
//
//	func init() {
//	    cache.RegisterResolver("redis", func(rawURL string) (cache.Cache, error) {
//	        // 解析 redis://host:6379/db 并构造 redis cache
//	    })
//	}
type Resolver func(rawURL string) (Cache, error)

// resolvers scheme → Resolver 注册表（线程安全懒初始化）。
var resolvers = map[string]Resolver{}

// RegisterResolver 注册 URL scheme → Resolver。
//
// 由 plugins 在 init() 中调用，主包零依赖。
// 重复注册同一 scheme 会被忽略（保留首次注册者，避免在多 plugin import 顺序不确定时被覆盖）。
func RegisterResolver(scheme string, r Resolver) {
	if scheme != "" && r != nil {
		if _, exists := resolvers[scheme]; !exists {
			resolvers[scheme] = r
		}
	}
}

// RegisteredResolvers 返回当前已注册的所有 scheme（用于诊断与文档）。
//
// 顺序不保证（map 遍历）；如需排序由调用方处理。
func RegisteredResolvers() map[string]struct{} {
	out := make(map[string]struct{}, len(resolvers))
	for k := range resolvers {
		out[k] = struct{}{}
	}
	return out
}

// resolveScheme 从 rawURL 提取 scheme（"redis://host:6379" → "redis"）。
//
// 无 "://" 时返回空字符串（视为未知 scheme）。
func resolveScheme(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx > 0 {
		return rawURL[:idx]
	}
	return ""
}

// NewFromURL 按 URL scheme 构造 Cache 实例。
//
// 行为：
//   - "" → 返回 (nil, nil)，调用方按需用默认实现兜底
//   - "scheme://..." → 查找 RegisterResolver 注册的解析器
//   - 未知 scheme → 返回 error（避免静默用错误实现误导生产）
//
// 与 app.resolveRegistry 的差异：
//   - resolveRegistry 默认 "" 走 memory（注册中心是必备组件）
//   - NewFromURL 默认 "" 返回 nil（cache 是可选组件，由调用方决定兜底策略）
func NewFromURL(rawURL string) (Cache, error) {
	if rawURL == "" {
		return nil, nil
	}
	scheme := resolveScheme(rawURL)
	if scheme == "" {
		return nil, fmt.Errorf("cache.NewFromURL: invalid URL %q (expected scheme://...)", rawURL)
	}
	r, ok := resolvers[scheme]
	if !ok {
		return nil, fmt.Errorf("cache.NewFromURL: unknown scheme %q (import the corresponding plugin, e.g. _ \"github.com/go-zeus/zeus/plugins/cache/redis\")", scheme)
	}
	return r(rawURL)
}
