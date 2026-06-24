package url

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// JoinPaths 合并url
func JoinPaths(absolutePath, relativePath string) string {
	if relativePath == "" {
		return absolutePath
	}
	//处理http
	scheme := ""
	url, err := url.Parse(absolutePath)
	if err == nil && url.Scheme != "" {
		scheme = url.Scheme + "://"
		absolutePath = strings.TrimPrefix(absolutePath, scheme)
	}
	finalPath := path.Join(absolutePath, relativePath)
	appendSlash := lastChar(relativePath) == '/' && lastChar(finalPath) != '/'
	if appendSlash {
		return finalPath + "/"
	}
	return scheme + finalPath
}

func lastChar(str string) uint8 {
	if str == "" {
		panic("The length of the string can't be 0")
	}
	return str[len(str)-1]
}

// SingleJoiningSlash 获取url中的第一个参数
func SingleJoiningSlash(a, b string) string {
	aSlash := strings.HasSuffix(a, "/")
	bSlash := strings.HasPrefix(b, "/")
	switch {
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	}
	return a + b
}

// URL 根据 *http.Request 拼出完整请求 URL（含 scheme）。
//
// scheme 按 r.TLS 是否非 nil 决定（https / http）。
func URL(r *http.Request) (URL string) {
	scheme := "http://"
	if r.TLS != nil {
		scheme = "https://"
	}
	return strings.Join([]string{scheme, r.Host}, "")
}

// ClientIP 获取ClientIP
//
// 优先级（参考 RFC 7239 / 主流 L7 LB 实践）：
//  1. X-Forwarded-For（AWS ALB / nginx 默认使用，逗号分隔的客户端链路）
//  2. Forwarded（RFC 7239 标准格式，for=192.0.2.43）
//  3. X-Real-Ip（部分代理 / 自定义场景使用）
//  4. RemoteAddr（TCP 直连兜底）
func ClientIP(r *http.Request) string {
	// 1. X-Forwarded-For：取第一个 IP（链路最远端是真实客户端）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip := strings.TrimSpace(strings.Split(xff, ",")[0])
		if ip != "" {
			return ip
		}
	}

	// 2. Forwarded（RFC 7239）：解析 for= 参数
	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		if ip := parseForwardedFor(fwd); ip != "" {
			return ip
		}
	}

	// 3. X-Real-Ip：单一 IP
	if ip := strings.TrimSpace(r.Header.Get("X-Real-Ip")); ip != "" {
		return ip
	}

	// 4. RemoteAddr：TCP 连接的对端
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return ip
	}
	return ""
}

// parseForwardedFor 解析 RFC 7239 Forwarded header 中的第一个 for= 参数
//
// Forwarded 格式示例：
//
//	Forwarded: for=192.0.2.43, for=198.51.100.17
//	Forwarded: for="192.0.2.43:8080"; proto=https; host=example.com
//	Forwarded: for="[2001:db8::1]:8080"
//
// 返回纯 IP（去端口、去引号、去 IPv6 括号）
func parseForwardedFor(forwarded string) string {
	parts := strings.Split(forwarded, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// 多个 proxy 链路用逗号分隔
		for _, item := range strings.Split(part, ",") {
			item = strings.TrimSpace(item)
			if !strings.HasPrefix(strings.ToLower(item), "for=") {
				continue
			}
			value := strings.TrimSpace(item[4:])
			// 去引号
			value = strings.Trim(value, "\"")
			// 去端口（IPv4:port 或 [IPv6]:port）
			if ip, _, err := net.SplitHostPort(value); err == nil {
				return ip
			}
			// 无端口的纯 IP（可能含 IPv6 括号）
			value = strings.Trim(value, "[]")
			if value != "" {
				return value
			}
		}
	}
	return ""
}
