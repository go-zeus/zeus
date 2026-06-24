package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/utils/uuid"
)

// isWebSocketUpgrade 检测 WebSocket 升级请求
// 标准：Connection 头包含 upgrade，Upgrade 头为 websocket（大小写不敏感）
func isWebSocketUpgrade(r *http.Request) bool {
	if !headerContains(r.Header, "Connection", "upgrade") {
		return false
	}
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// isSSERequest 检测 SSE 请求
// 标准：Accept 头包含 text/event-stream
func isSSERequest(r *http.Request) bool {
	return headerContains(r.Header, "Accept", "text/event-stream")
}

// headerContains 检查 header 字段是否包含指定 token（CSV 解析，大小写不敏感）
func headerContains(h http.Header, key, token string) bool {
	values := h[key]
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// handleHTTP 处理普通 HTTP/HTTPS 反向代理
func (p *proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	target, err := p.selector.Pick(r)
	if err != nil {
		p.errorHandler(w, r, err)
		return
	}

	// 浅拷贝 target 避免 Director 修改共享对象
	targetCopy := *target

	rp := &httputil.ReverseProxy{
		Director:       p.composeDirector(&targetCopy),
		ModifyResponse: p.toModifyResponse(),
		ErrorHandler:   p.errorHandler,
		Transport:      p.transport,
	}
	rp.ServeHTTP(w, r)
}

// composeDirector 组合默认 Director 和用户自定义 Director
// 默认行为：
//   - 设置 scheme/host
//   - 追加 X-Forwarded-For
//   - 注入 X-Real-IP
//   - 缺失 X-Request-ID 时生成
func (p *proxy) composeDirector(target *url.URL) func(*http.Request) {
	return func(req *http.Request) {
		// 设置目标 scheme/host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		// 追加 X-Forwarded-For（保留链）
		clientIP := remoteIP(req)
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Real-IP", remoteIP(req))

		// 注入 X-Request-ID（缺失时生成）
		if req.Header.Get("X-Request-ID") == "" {
			req.Header.Set("X-Request-ID", uuid.New())
		}

		// 用户自定义 Director 叠加
		if p.userDirector != nil {
			p.userDirector(target, req)
		}
	}
}

// toModifyResponse 将 ResponseRewriter 转换为 httputil 的 ModifyResponse 签名
func (p *proxy) toModifyResponse() func(*http.Response) error {
	if p.responseRewriter == nil {
		return nil
	}
	return func(resp *http.Response) error {
		if err := p.responseRewriter(resp); err != nil {
			log.Info("proxy: response rewrite error: %v", err)
			return err
		}
		return nil
	}
}

// remoteIP 提取客户端 IP（去除端口）
// 使用 net.SplitHostPort 正确处理 IPv6 地址（如 [::1]:8080）
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// 没有端口或格式异常，原样返回
		return r.RemoteAddr
	}
	return host
}
