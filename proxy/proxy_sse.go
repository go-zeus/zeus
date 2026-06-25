package proxy

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-zeus/zeus/log"
)

// handleSSE 处理 SSE (Server-Sent Events) 反向代理
//
// 关键差异（与普通 HTTP 反代相比）：
//  1. 禁用响应缓冲，每读一段数据立即 Flush 到客户端
//  2. 强制 Cache-Control: no-cache、Connection: keep-alive
//  3. 串行 read-write-flush 保证事件顺序（不可并发 copy）
//  4. 客户端断开（context 取消）时关闭后端连接
func (p *proxy) handleSSE(w http.ResponseWriter, r *http.Request) {
	target, err := p.selector.Pick(r)
	if err != nil {
		p.errorHandler(w, r, err)
		return
	}

	// 构造转发请求
	upReq := r.Clone(r.Context())
	upReq.URL.Scheme = target.Scheme
	upReq.URL.Host = target.Host
	upReq.RequestURI = ""

	resp, err := p.transport.RoundTrip(upReq)
	if err != nil {
		p.errorHandler(w, r, err)
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	copyHeaders(w.Header(), resp.Header)
	// 强制 SSE 关键 header
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 移除 Content-Length，避免长度不匹配导致客户端等待
	w.Header().Del("Content-Length")

	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush() // 立即推送头部
	}

	// 串行 read-write-flush 循环
	buf := make([]byte, 4096)
	for {
		// 客户端断开检查
		if err := r.Context().Err(); err != nil {
			return
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				log.Info("proxy: sse client write failed: %v", werr)
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			// io.EOF 是正常关闭，其他错误记录日志
			if !errors.Is(readErr, io.EOF) {
				log.Info("proxy: sse backend read end: %v", readErr)
			}
			return
		}
	}
}

// copyHeaders 浅拷贝 HTTP 头
func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
