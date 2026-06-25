package server

import (
	"context"
	"testing"
)

// ---- mock server ----

type mockServer struct {
	protocol string
	endpoint string
	started  bool
	stopped  bool
}

func (m *mockServer) Protocol() string              { return m.protocol }
func (m *mockServer) Endpoint() string              { return m.endpoint }
func (m *mockServer) Start(_ context.Context) error { m.started = true; return nil }
func (m *mockServer) Stop(_ context.Context) error  { m.stopped = true; return nil }

// ---- tests ----

// TestServerInterface 验证 Server 接口方法可调用
func TestServerInterface(t *testing.T) {
	m := &mockServer{protocol: "http", endpoint: "0.0.0.0:9090"}
	var srv Server = m

	if err := srv.Start(context.Background()); err != nil {
		t.Errorf("Start error: %v", err)
	}
	if !m.started {
		t.Error("Start was not called")
	}

	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("Stop error: %v", err)
	}
	if !m.stopped {
		t.Error("Stop was not called")
	}
}

// TestServerProtocol 验证 Protocol() 返回值
func TestServerProtocol(t *testing.T) {
	m := &mockServer{protocol: "grpc", endpoint: "0.0.0.0:9000"}
	if m.Protocol() != "grpc" {
		t.Errorf("Protocol() = %q, want %q", m.Protocol(), "grpc")
	}
}

// TestServerEndpoint 验证 Endpoint() 返回值
func TestServerEndpoint(t *testing.T) {
	m := &mockServer{protocol: "http", endpoint: "127.0.0.1:8080"}
	if m.Endpoint() != "127.0.0.1:8080" {
		t.Errorf("Endpoint() = %q, want %q", m.Endpoint(), "127.0.0.1:8080")
	}
}
