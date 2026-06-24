package http

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// getFreePort 获取一个可用的随机端口
func getFreePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestNewHTTP_DefaultPort 验证 NewHTTP() 默认端口为 8080
func TestNewHTTP_DefaultPort(t *testing.T) {
	s := NewHTTP().(*httpServer)
	if s.port != DefaultPort {
		t.Errorf("默认端口 = %d, want %d", s.port, DefaultPort)
	}
}

// TestNewHTTP_CustomPort 验证 NewHTTP(Port(3000)) 端口为 3000
func TestNewHTTP_CustomPort(t *testing.T) {
	s := NewHTTP(Port(3000)).(*httpServer)
	if s.port != 3000 {
		t.Errorf("自定义端口 = %d, want %d", s.port, 3000)
	}
}

// TestNewHTTP_CustomIp 验证自定义 IP 和端口 0 时的 Endpoint 格式
func TestNewHTTP_CustomIp(t *testing.T) {
	// 端口为 0 时会被默认值覆盖为 8080
	s := NewHTTP(IP("127.0.0.1"), Port(0)).(*httpServer)
	if s.port != DefaultPort {
		t.Errorf("端口 = %d, want %d", s.port, DefaultPort)
	}
	endpoint := s.Endpoint()
	want := "127.0.0.1:8080"
	if endpoint != want {
		t.Errorf("Endpoint() = %q, want %q", endpoint, want)
	}
}

// TestProtocol 验证 Protocol() 返回 "http"
func TestProtocol(t *testing.T) {
	s := NewHTTP()
	if got := s.Protocol(); got != ProtocolName {
		t.Errorf("Protocol() = %q, want %q", got, ProtocolName)
	}
}

// TestEndpoint 验证 Endpoint() 在有 IP 和无 IP 时的格式
func TestEndpoint(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{
			name: "无IP默认端口",
			opts: nil,
			want: ":8080",
		},
		{
			name: "有IP自定义端口",
			opts: []Option{IP("192.168.1.1"), Port(9090)},
			want: "192.168.1.1:9090",
		},
		{
			name: "有IP默认端口",
			opts: []Option{IP("0.0.0.0")},
			want: "0.0.0.0:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewHTTP(tt.opts...).(*httpServer)
			if got := s.Endpoint(); got != tt.want {
				t.Errorf("Endpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDefaultHandler 验证 DefaultHandler 返回 "welcome to zeus!"
func TestDefaultHandler(t *testing.T) {
	handler := DefaultHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	want := "welcome to zeus!"
	if body != want {
		t.Errorf("响应体 = %q, want %q", body, want)
	}
}

// TestStartAndStop 启动服务器并发送请求，然后停止
func TestStartAndStop(t *testing.T) {
	port := getFreePort()
	s := NewHTTP(IP("127.0.0.1"), Port(port)).(*httpServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动服务器
	go s.Start(ctx)

	// 等待服务器启动
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitForServer(addr, 2*time.Second); err != nil {
		t.Fatalf("服务器未在超时内启动: %v", err)
	}

	// 发送请求验证服务正常
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "welcome to zeus!") {
		t.Errorf("响应体 = %q, want 包含 %q", string(body), "welcome to zeus!")
	}

	// 停止服务器
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	if err := s.Stop(stopCtx); err != nil {
		t.Errorf("Stop() 错误: %v", err)
	}
}

// TestHealthCheckRoutes 启动服务器验证健康检查路由
func TestHealthCheckRoutes(t *testing.T) {
	port := getFreePort()
	s := NewHTTP(IP("127.0.0.1"), Port(port)).(*httpServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Start(ctx)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitForServer(addr, 2*time.Second); err != nil {
		t.Fatalf("服务器未在超时内启动: %v", err)
	}

	// 测试 /health 路由
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("请求 /health 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health 状态码 = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// 停止服务器
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	s.Stop(stopCtx)
}

// TestCustomMux 验证自定义 Mux 被使用
func TestCustomMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/custom", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom handler"))
	})

	port := getFreePort()
	s := NewHTTP(Mux(mux), IP("127.0.0.1"), Port(port)).(*httpServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Start(ctx)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitForServer(addr, 2*time.Second); err != nil {
		t.Fatalf("服务器未在超时内启动: %v", err)
	}

	// 请求自定义路由
	resp, err := http.Get("http://" + addr + "/custom")
	if err != nil {
		t.Fatalf("请求 /custom 失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "custom handler" {
		t.Errorf("自定义路由响应 = %q, want %q", string(body), "custom handler")
	}

	// 使用自定义 Mux 时不应该注册健康检查路由
	healthResp, err := http.Get("http://" + addr + "/health")
	if err == nil {
		healthResp.Body.Close()
		// 自定义 Mux 下 /health 应该返回 404（未注册）
		if healthResp.StatusCode != http.StatusNotFound {
			t.Errorf("自定义 Mux 下 /health 状态码 = %d, want %d", healthResp.StatusCode, http.StatusNotFound)
		}
	}

	// 停止服务器
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	s.Stop(stopCtx)
}

// TestStop_IdleServer 停止未启动的服务器，应返回 nil
// 注意：http.Server.Shutdown() 在服务器未启动时返回 nil，而非 http.ErrServerClosed
// http.ErrServerClosed 只在服务器已在 Serve 中被 Shutdown 关闭时才返回
func TestStop_IdleServer(t *testing.T) {
	s := NewHTTP()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	err := s.Stop(stopCtx)
	if err != nil {
		t.Errorf("未启动的服务器 Stop() 错误 = %v, want nil", err)
	}
}

// waitForServer 轮询等待服务器就绪
func waitForServer(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("服务器 %s 在 %v 内未就绪", addr, timeout)
}
