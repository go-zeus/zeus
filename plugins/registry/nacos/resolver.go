// URL scheme resolver 注册：nacos:// → nacos.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/registry/nacos"
//	// app.Run 或 app.NewApp 中传入 cfg.Registry = "nacos://..."
//
// URL 格式：
//   - nacos://host:8848                                 （单 server）
//   - nacos://h1:8848,h2:8848,h3:8848                   （多 server cluster）
//   - nacos://host:8848?namespace=prod&group=BIZ_GROUP  （带鉴权/命名空间）
//   - nacos://user:pass@host:8848                       （用户名密码）
//
// query 参数：
//   - namespace：命名空间 ID
//   - group：Group 名（默认 DEFAULT_GROUP）
//   - ak / sk：阿里云 ACM 风格鉴权
//
// 不支持的参数静默忽略（前向兼容）。

package nacos

import (
	"net/url"
	"strings"

	"github.com/go-zeus/zeus/app"
	"github.com/go-zeus/zeus/registry"
)

func init() {
	app.RegisterRegistryResolver("nacos", resolveFromURL)
}

// resolveFromURL 把 "nacos://..." URL 解析为 nacos.New(...) 实例
func resolveFromURL(rawURL string) (registry.Registrar, error) {
	opts := parseURLOptions(rawURL)
	return New(opts...), nil
}

// parseURLOptions 把 nacos:// URL 解析为 New() 的 Option 列表
//
// 单独抽出便于单元测试
func parseURLOptions(rawURL string) []Option {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "nacos://") {
		return []Option{WithServer(rawURL, DefaultServerPort)}
	}

	body := rawURL[len("nacos://"):]

	var hostPart, queryPart string
	if idx := strings.Index(body, "?"); idx >= 0 {
		hostPart = body[:idx]
		queryPart = body[idx+1:]
	} else {
		hostPart = body
	}

	var opts []Option

	// 处理 user:pass@host 形式（鉴权）
	if idx := strings.LastIndex(hostPart, "@"); idx >= 0 {
		authPart := hostPart[:idx]
		hostPart = hostPart[idx+1:]

		if cIdx := strings.Index(authPart, ":"); cIdx >= 0 {
			opts = append(opts, WithCredentials(authPart[:cIdx], authPart[cIdx+1:]))
		} else if authPart != "" {
			opts = append(opts, WithCredentials(authPart, ""))
		}
	}

	// 多 server：逗号分隔
	hosts := splitAndTrim(hostPart, ",")
	for _, h := range hosts {
		serverHost, serverPort := splitHostPort(h, DefaultServerPort)
		if serverHost != "" {
			opts = append(opts, WithServer(serverHost, serverPort))
		}
	}

	// query 参数
	if queryPart != "" {
		query, err := url.ParseQuery(queryPart)
		if err == nil {
			if v := query.Get("namespace"); v != "" {
				opts = append(opts, WithNamespace(v))
			}
			if v := query.Get("group"); v != "" {
				opts = append(opts, WithGroup(v))
			}
			if ak, sk := query.Get("ak"), query.Get("sk"); ak != "" && sk != "" {
				opts = append(opts, WithAccessKey(ak, sk))
			}
		}
	}

	return opts
}

// splitAndTrim 分割字符串并 trim 每段
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

// splitHostPort 把 host[:port] 拆为 host 和 port（缺失 port 时用 defaultPort）
func splitHostPort(s string, defaultPort int) (string, int) {
	if s == "" {
		return "", 0
	}
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		port, err := simpleAtoi(s[idx+1:])
		if err == nil && port > 0 {
			return s[:idx], port
		}
	}
	return s, defaultPort
}

// simpleAtoi 轻量整数解析（避免顶部 import strconv）
func simpleAtoi(s string) (int, error) {
	if s == "" {
		return 0, errInvalidPort
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidPort
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var errInvalidPort = &invalidPortError{}

type invalidPortError struct{}

func (e *invalidPortError) Error() string { return "invalid port" }
