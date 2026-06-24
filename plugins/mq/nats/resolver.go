// URL scheme resolver 注册：nats:// → nats.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/mq/nats"
//	b, _ := mq.NewBrokerFromURL("nats://127.0.0.1:4222?timeout=5s")
//
// URL 格式（NATS 原生支持）：
//   - nats://host:port                          （单服务器）
//   - nats://h1:4222,h2:4222,h3:4222            （多服务器 cluster）
//   - nats://user:pass@host:4222                （带认证）
//
// query 参数：
//   - timeout：连接超时（time.Duration 字符串，如 5s）
//
// 不支持的参数静默忽略（前向兼容）。
// 高级配置（TLS / ReconnectWait / MaxReconnects）请用 nats.New + WithNATSOptions。
//
// 实现说明：NATS 原生连接字符串支持逗号分隔多 host，
// 但 Go 标准库 net/url 不支持解析逗号 host，故本 resolver 不做 URL parse 重建，
// 直接把原始 URL 透传给 nats.Connect，仅提取 query 参数。

package nats

import (
	"net/url"
	"strings"
	"time"

	"github.com/go-zeus/zeus/mq"
)

func init() {
	mq.RegisterResolver("nats", resolveFromURL)
}

// resolveFromURL 把 "nats://..." URL 解析为 nats.New(...) 实例
func resolveFromURL(rawURL string) (mq.Broker, error) {
	opts := parseURLOptions(rawURL)
	return New(opts...)
}

// parseURLOptions 把 nats:// URL 解析为 New() 的 Option 列表
//
// 单独抽出便于单元测试（不实际建立 NATS 连接）
//
// 解析策略：
//  1. 用 url.Parse 提取 query 参数（如果 URL 合法）
//  2. URL 连接字符串原样透传给 NATS（保留多 host 逗号语法）
//  3. url.Parse 失败时（如多 host 形式），跳过 query 解析但仍透传 URL
func parseURLOptions(rawURL string) []Option {
	var opts []Option

	if u, err := url.Parse(rawURL); err == nil && u.Scheme == "nats" {
		// 单服务器 + 合法 URL：从 User/Host 重建连接字符串
		opts = append(opts, WithURL(reconstructConnURL(u)))
		if v := u.Query().Get("timeout"); v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				opts = append(opts, WithConnectTimeout(d))
			}
		}
		return opts
	}

	// 多服务器（含逗号）或解析失败：直接透传 rawURL 给 NATS
	opts = append(opts, WithURL(rawURL))
	return opts
}

// reconstructConnURL 从 url.URL 重建干净的 nats:// 连接字符串（剥离 query 参数）
func reconstructConnURL(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "4222"
	}

	if u.User != nil {
		user := u.User.Username()
		pass, hasPass := u.User.Password()
		if hasPass {
			return "nats://" + user + ":" + pass + "@" + host + ":" + port
		}
		if user != "" {
			return "nats://" + user + "@" + host + ":" + port
		}
	}
	return "nats://" + host + ":" + port
}

// 多服务器场景检查（保留 strings 引用，避免未来扩展时丢失 import）
var _ = strings.Contains
