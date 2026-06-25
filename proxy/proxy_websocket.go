package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/go-zeus/zeus/log"
)

// handleWebSocket 处理 WebSocket 反向代理
//
// 实现策略：nginx 风格的 raw 透传
//  1. Hijack 客户端 TCP 连接
//  2. 拨号后端 TCP 连接
//  3. 透传原始 HTTP Upgrade 握手到后端
//  4. 双向 io.Copy 透传后续 WS 帧（不解析 RFC6455 帧）
//
// 此方式符合 KISS：不引入 gorilla/websocket 依赖，ping/pong 透传自然处理。
// 限制：无法介入 WS 消息层（限速/改写），如有此需求可后续独立 plugins/proxy/ws。
func (p *proxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	target, err := p.selector.Pick(r)
	if err != nil {
		p.errorHandler(w, r, err)
		return
	}

	// Hijack 客户端连接
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy: hijack not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		log.Info("proxy: hijack client failed: %v", err)
		return
	}
	defer clientConn.Close()

	// 拨号后端
	backendAddr := target.Host
	backend, err := net.Dial("tcp", backendAddr)
	if err != nil {
		log.Info("proxy: dial backend %s failed: %v", backendAddr, err)
		writeRawStatus(clientBuf.Writer, http.StatusBadGateway, "Bad Gateway")
		return
	}
	defer backend.Close()

	// 透传原始 Upgrade 握手到后端
	// r.Write 写入 Method/URL/Host/Header，符合 HTTP/1.1 Upgrade 规范
	if err := r.Write(backend); err != nil {
		log.Info("proxy: write upgrade request to backend failed: %v", err)
		return
	}

	// 双向 io.Copy
	// 任一方向出错或 EOF 即关闭两端，goroutine 退出
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(backend, clientConn)
		_ = closeWrite(backend)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, backend)
		_ = closeWrite(clientConn)
	}()
	wg.Wait()
}

// writeRawStatus 在 hijack 后通过 bufio.Writer 写入 HTTP 响应
// 必须手动 Flush，否则数据停留在缓冲区
func writeRawStatus(bw *bufio.Writer, code int, status string) {
	_, _ = fmt.Fprintf(bw, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", code, status)
	_ = bw.Flush()
}

// closeWrite 尝试关闭连接的写端（TCP FIN），支持则关闭，否则跳过
func closeWrite(c net.Conn) error {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
