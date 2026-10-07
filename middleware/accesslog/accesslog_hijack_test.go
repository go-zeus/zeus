package accesslog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHTTPMiddleware_HijackPassthrough 验证经 accesslog 包装后 handler 拿到的
// ResponseWriter 仍实现 http.Hijacker 并可透传劫持。
// 回归锚：包装丢失 Hijacker 曾令代理的 WebSocket 升级全数 500
//（proxy: hijack not supported）。
func TestHTTPMiddleware_HijackPassthrough(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("accesslog 包装丢失了 http.Hijacker 接口")
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack 透传失败: %v", err)
		}
		// 连接已裸：手写响应后关闭，客户端按正常 200 收尾。
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
		_ = buf.Flush()
		_ = conn.Close()
	})
	srv := httptest.NewServer(HTTPMiddleware(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", resp.StatusCode)
	}
}
