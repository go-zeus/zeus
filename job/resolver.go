// URL scheme resolver（与 cache/mq/database 包同构）。
//
// 设计目的：让 job 调度器也能通过 URL 字符串切换实现，与其他功能域一致。
//
// 用户在配置中传入：
//
//	"interval://"                      → job/interval（内置，固定间隔）
//	"interval://?err=custom"           → interval + 自定义 ErrorHandler（待补）
//	"cron://"                           → plugins/job/cron（cron 表达式）
//	"cron://?seconds=true&loc=UTC"     → cron + 启用秒字段 + 时区
//
// plugins 在 init() 中调用 job.RegisterResolver 注册自己的 scheme，
// 主包零依赖（不直接 import plugins/job/cron 等第三方包）。

package job

import (
	"fmt"
	"strings"
)

// Resolver 把 URL 字符串解析为 Scheduler 实例。
//
// 由内置实现 / plugins 在 init() 中通过 RegisterResolver 注册。
// 例如 job/interval:
//
//	func init() {
//	    job.RegisterResolver("interval", func(rawURL string) (job.Scheduler, error) {
//	        return New(), nil
//	    })
//	}
type Resolver func(rawURL string) (Scheduler, error)

// resolvers scheme → Resolver 注册表（线程安全懒初始化）。
var resolvers = map[string]Resolver{}

// RegisterResolver 注册 URL scheme → Resolver。
//
// 由内置实现 / plugins 在 init() 中调用，主包零依赖。
// 重复注册同一 scheme 会被忽略（保留首次注册者，避免在多 plugin import 顺序不确定时被覆盖）。
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

// resolveScheme 从 rawURL 提取 scheme（"cron://?x=y" → "cron"）。
//
// 无 "://" 时返回空字符串（视为未知 scheme）。
func resolveScheme(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx > 0 {
		return rawURL[:idx]
	}
	return ""
}

// NewSchedulerFromURL 按 URL scheme 构造 Scheduler 实例。
//
// 行为：
//   - "" → 返回 (nil, nil)，调用方按需用默认实现兜底
//   - "scheme://..." → 查找 RegisterResolver 注册的解析器
//   - 未知 scheme → 返回 error（避免静默用错误实现误导生产）
//
// 与 cache.NewFromURL 一致：默认 "" 返回 nil（job 是可选组件，由调用方决定兜底策略）。
func NewSchedulerFromURL(rawURL string) (Scheduler, error) {
	if rawURL == "" {
		return nil, nil
	}
	scheme := resolveScheme(rawURL)
	if scheme == "" {
		return nil, fmt.Errorf("job.NewSchedulerFromURL: invalid URL %q (expected scheme://...)", rawURL)
	}
	r, ok := resolvers[scheme]
	if !ok {
		return nil, fmt.Errorf("job.NewSchedulerFromURL: unknown scheme %q (import the corresponding implementation, e.g. _ \"github.com/go-zeus/zeus/job/interval\" or _ \"github.com/go-zeus/zeus/plugins/job/cron\")", scheme)
	}
	return r(rawURL)
}
