package app

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/go-zeus/zeus/server"
)

// mockServer 用于测试的模拟服务器
type mockServer struct {
	started bool
	stopped bool
	startCh chan struct{}
}

func newMockServer() *mockServer {
	return &mockServer{startCh: make(chan struct{})}
}

func (m *mockServer) Protocol() string { return "mock" }
func (m *mockServer) Endpoint() string { return "127.0.0.1:0" }
func (m *mockServer) Start(ctx context.Context) error {
	m.started = true
	close(m.startCh)
	<-ctx.Done()
	return nil
}
func (m *mockServer) Stop(ctx context.Context) error {
	m.stopped = true
	return nil
}

// 编译期检查 mockServer 实现了 server.Server
var _ server.Server = (*mockServer)(nil)

func TestNew(t *testing.T) {
	a := New()
	if a == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_WithServer(t *testing.T) {
	s := newMockServer()
	a := New(WithServer(s))
	if a == nil {
		t.Fatal("New returned nil")
	}
}

func TestApp_Run_CloseSignal(t *testing.T) {
	s := newMockServer()
	a := New(WithServer(s))

	closeCh := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- a.Run(closeCh)
	}()

	// 等待 server 启动
	select {
	case <-s.startCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start in time")
	}

	// 发送关闭信号
	close(closeCh)

	if err := <-errCh; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !s.stopped {
		t.Fatal("server should have been stopped")
	}
}

func TestApp_NoServer_ReturnsError(t *testing.T) {
	// 不传 WithServer 时 Run 返回明确错误（不再依赖已废弃的 server.DefaultServer）
	a := New()
	if a == nil {
		t.Fatal("New returned nil")
	}
	err := a.Run(make(chan struct{}))
	if err == nil {
		t.Fatal("Run without server should return error (DefaultServer 已废弃)")
	}
}

// TestApp_Run_OSSignal_StopCalled 验证收到 OS 信号时 server.Stop() 仍被调用
// 这是生产环境关键路径：原来 eg.Wait() 错误返回会跳过 Stop 循环
func TestApp_Run_OSSignal_StopCalled(t *testing.T) {
	s := newMockServer()
	a := New(WithServer(s))

	// 永不关闭的 close 通道，确保只有信号路径触发退出
	closeCh := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- a.Run(closeCh)
	}()

	// 等待 server 启动
	select {
	case <-s.startCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start in time")
	}

	// 发送 SIGINT 信号
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess error: %v", err)
	}
	_ = p.Signal(syscall.SIGINT)

	// 应当返回非 nil 错误（信号路径）
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error on signal exit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return in time")
	}

	// 关键：信号路径下也必须调用 Stop()
	if !s.stopped {
		t.Fatal("server.Stop must be called even when signal triggers shutdown")
	}
}
