// URL scheme resolver（与 cache/database.RegisterResolver 同构）。
//
// 设计目的：让 mq 也能通过 URL 字符串切换实现。
//
// 用户在 L1/L2/L3 配置中传入：
//
//	"memory://?buffer=128"               → mq/memory（进程内事件总线）
//	"kafka://host:9092?group=g1"          → plugins/mq/kafka（待补）
//	"nats://host:4222"                    → plugins/mq/nats（待补）
//	"redis://host:6379/0?stream=orders"   → plugins/mq/redis（待补）
//
// plugins 在 init() 中调用 mq.RegisterResolver 注册自己的 scheme。

package mq

import (
	"fmt"
	"strings"
)

// Resolver 把 URL 字符串解析为 Broker 实例。
//
// 由 plugins 在 init() 中通过 RegisterResolver 注册。
type Resolver func(rawURL string) (Broker, error)

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

// resolveScheme 从 rawURL 提取 scheme。
func resolveScheme(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx > 0 {
		return rawURL[:idx]
	}
	return ""
}

// NewBrokerFromURL 按 URL scheme 构造 Broker 实例。
//
// 行为：
//   - "" → 返回 (nil, nil)，调用方按需用默认实现兜底
//   - "scheme://..." → 查找 RegisterResolver 注册的解析器
//   - 未知 scheme → 返回 error
//
// 命名说明：函数叫 NewBrokerFromURL（而非 NewFromURL）以避免与 Publisher.NewFromURL / Subscriber.NewFromURL
// 未来可能的扩展混淆。当前 mq 主包不直接提供 Publisher/Subscriber 单独构造入口（与 cache 不同）。
func NewBrokerFromURL(rawURL string) (Broker, error) {
	if rawURL == "" {
		return nil, nil
	}
	scheme := resolveScheme(rawURL)
	if scheme == "" {
		return nil, fmt.Errorf("mq.NewBrokerFromURL: invalid URL %q (expected scheme://...)", rawURL)
	}
	r, ok := resolvers[scheme]
	if !ok {
		return nil, fmt.Errorf("mq.NewBrokerFromURL: unknown scheme %q (import the corresponding plugin, e.g. _ \"github.com/go-zeus/zeus/mq/memory\")", scheme)
	}
	return r(rawURL)
}
