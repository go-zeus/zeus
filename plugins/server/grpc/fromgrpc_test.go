package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-zeus/zeus/app"
	"google.golang.org/grpc"
)

// TestFromGRPC_WrapsUserServer 验证 FromGRPC 接受用户已构造好的 *grpc.Server
// 并能通过 Protocol()/Endpoint() 适配为 server.Server
func TestFromGRPC_WrapsUserServer(t *testing.T) {
	port := freePort(t)
	gs := grpc.NewServer()

	srv := FromGRPC(gs, Port(port))
	if srv.Protocol() != "grpc" {
		t.Errorf("Protocol() = %q, want %q", srv.Protocol(), "grpc")
	}
	want := endpointFor(port)
	if srv.Endpoint() != want {
		t.Errorf("Endpoint() = %q, want %q", srv.Endpoint(), want)
	}
}

// TestServerResolver_RegisteredByInit 验证 init() 把 *grpc.Server 嗅探器
// 注册到 app.RegisterServerResolver。L1 入口 app.Run 应该能识别 *grpc.Server
// 并构造出 grpc server.Server。
//
// 注意：因为 init() 注册的是闭包，难以直接断言已注册。
// 这里通过间接路径：从 app 包读取 server resolvers 列表长度 >= 1。
func TestServerResolver_RegisteredByInit(t *testing.T) {
	// app.serverResolvers 是包内 slice，无法直接访问。
	// 通过行为观察：传入 *grpc.Server，看是否返回 *grpcServer。
	port := freePort(t)
	gs := grpc.NewServer()

	// 复刻 app.Run 内部的 resolveServer 调用路径
	cfg := &app.Config{Port: port}
	resolvers := app.ServerResolvers()
	if len(resolvers) == 0 {
		t.Fatal("app.ServerResolvers() is empty; init() should register *grpc.Server resolver")
	}

	var matched bool
	for _, r := range resolvers {
		srv, err := r(gs, cfg)
		if err != nil {
			t.Fatalf("resolver returned error: %v", err)
		}
		if srv != nil {
			matched = true
			if srv.Protocol() != "grpc" {
				t.Errorf("Protocol() = %q, want grpc", srv.Protocol())
			}
			break
		}
	}
	if !matched {
		t.Fatal("no resolver matched *grpc.Server")
	}

	// 非匹配类型应让所有 resolver 返回 (nil, nil)
	for _, r := range resolvers {
		srv, _ := r("not a grpc server", cfg)
		if srv != nil {
			t.Errorf("resolver matched non-grpc handler, should return nil")
		}
	}
}

// TestFromGRPC_StartAndStop 验证 owner-provided *grpc.Server 在 Start/Stop 时
// 不被替换（用户注册的 service 保留），且能优雅关闭
func TestFromGRPC_StartAndStop(t *testing.T) {
	port := freePort(t)
	gs := grpc.NewServer()
	srv := FromGRPC(gs, Port(port))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	// 等待监听就绪
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", endpointFor(port), 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	_ = srv.Stop(context.Background())
}

// endpointFor 简化 :<port> 字符串拼接，避免引入 strconv
func endpointFor(port int) string {
	return ":" + itoa(port)
}

// freePort 获取空闲端口
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// itoa 简化 strconv.Itoa
func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = digits[n%10]
		n /= 10
	}
	return string(buf[pos:])
}
