// URL scheme resolver 注册：etcd:// → etcd.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/registry/etcd"
//	// app.Run 或 app.NewApp 中传入 cfg.Registry = "etcd://..."
//
// URL 格式：
//   - etcd://127.0.0.1:2379                          （单 endpoint）
//   - etcd://h1:2379,h2:2379,h3:2379                  （多 endpoint cluster）
//   - etcd://user:pass@host:2379                      （用户名密码鉴权）
//   - etcd://host:2379?ttl=10s&prefix=/zeus/&timeout=5s
//
// query 参数：
//   - ttl：lease TTL（time.Duration 字符串，最小 5s，默认 30s）
//   - prefix：key 前缀（默认 /zeus/services/）
//   - timeout：拨号超时（time.Duration 字符串，默认 30s）
//
// endpoint 不带端口时补默认 :2379；不支持的 query 参数静默忽略（前向兼容）。

package etcd

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-zeus/zeus/app"
	"github.com/go-zeus/zeus/registry"
)

const defaultScheme = "etcd://"
const defaultPort = "2379"

func init() {
	app.RegisterRegistryResolver("etcd", resolveFromURL)
}

// resolveFromURL 把 "etcd://..." URL 解析为 etcd.New(...) 实例
func resolveFromURL(rawURL string) (registry.Registrar, error) {
	opts, err := parseURLOptions(rawURL)
	if err != nil {
		return nil, err
	}
	return New(opts...), nil
}

// parseURLOptions 把 etcd:// URL 解析为 New() 的 Option 列表
//
// 手动解析（不用 url.Parse 整体）以正确处理多 endpoint 逗号语法，
// 与 plugins/registry/nacos 的解析模式保持一致。
func parseURLOptions(rawURL string) ([]Option, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, defaultScheme) {
		return nil, fmt.Errorf("etcd: invalid URL %q (expected etcd://...)", rawURL)
	}

	body := rawURL[len(defaultScheme):]

	var hostPart, queryPart string
	if idx := strings.Index(body, "?"); idx >= 0 {
		hostPart = body[:idx]
		queryPart = body[idx+1:]
	} else {
		hostPart = body
	}

	var opts []Option

	// 鉴权：user:pass@host
	if idx := strings.LastIndex(hostPart, "@"); idx >= 0 {
		authPart := hostPart[:idx]
		hostPart = hostPart[idx+1:]
		if cIdx := strings.Index(authPart, ":"); cIdx >= 0 {
			opts = append(opts, WithCredentials(authPart[:cIdx], authPart[cIdx+1:]))
		} else if authPart != "" {
			opts = append(opts, WithCredentials(authPart, ""))
		}
	}

	// endpoints：逗号分隔；无端口的补 :2379
	endpoints := splitAndTrim(hostPart, ",")
	for i, ep := range endpoints {
		if ep != "" && !strings.Contains(ep, ":") {
			endpoints[i] = ep + ":" + defaultPort
		}
	}
	if len(endpoints) > 0 {
		opts = append(opts, WithEndpoints(endpoints...))
	}

	// query 参数
	if queryPart != "" {
		query, err := url.ParseQuery(queryPart)
		if err == nil {
			if v := query.Get("ttl"); v != "" {
				if d, e := time.ParseDuration(v); e == nil && d > 0 {
					opts = append(opts, WithTTL(d))
				}
			}
			if v := query.Get("prefix"); v != "" {
				opts = append(opts, WithPrefix(v))
			}
			if v := query.Get("timeout"); v != "" {
				if d, e := time.ParseDuration(v); e == nil && d > 0 {
					opts = append(opts, WithDialTimeout(d))
				}
			}
		}
	}

	return opts, nil
}

// splitAndTrim 分割字符串并 trim 每段（与 plugins/registry/nacos 同名工具一致）
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
