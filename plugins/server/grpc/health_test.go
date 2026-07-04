package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// startAndWait 启动 grpcServer 并等待其就绪（或超时）。
// 返回的 stop func 会优雅关闭并等待 Start 返回。
func startAndWait(t *testing.T, s *grpcServer) (string, func()) {
	t.Helper()
	port := freePort(t)
	s.ip = "127.0.0.1"
	s.port = port

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Start(ctx)
		close(done)
	}()

	// 等待 server 就绪：尝试 dial 直到成功或超时
	endpoint := "127.0.0.1:" + itoa(port)
	deadline := time.Now().Add(2 * time.Second)
	var conn *grpc.ClientConn
	for time.Now().Before(deadline) {
		c, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			conn = c
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if conn == nil {
		cancel()
		t.Fatalf("server did not become ready on %s", endpoint)
	}

	stop := func() {
		_ = conn.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not return after cancel")
		}
	}
	return endpoint, stop
}

// --- 字段默认值单元测试 ---

func TestHealth_DefaultEnabled_NewGRPC(t *testing.T) {
	s := NewGRPC().(*grpcServer)
	if !s.healthEnabled {
		t.Error("NewGRPC: healthEnabled should default to true")
	}
}

func TestHealth_DisabledByOption(t *testing.T) {
	s := NewGRPC(WithoutHealth()).(*grpcServer)
	if s.healthEnabled {
		t.Error("WithoutHealth did not disable health")
	}
}

func TestHealth_DefaultDisabled_FromGRPC(t *testing.T) {
	gs := grpc.NewServer()
	defer gs.Stop()
	s := FromGRPC(gs).(*grpcServer)
	if s.healthEnabled {
		t.Error("FromGRPC: healthEnabled should default to false")
	}
}

func TestHealth_EnabledFromGRPC(t *testing.T) {
	gs := grpc.NewServer()
	defer gs.Stop()
	s := FromGRPC(gs, WithHealth()).(*grpcServer)
	if !s.healthEnabled {
		t.Error("WithHealth did not enable health on FromGRPC")
	}
}

// --- 集成测试：真实 RPC 调用 health check ---

// TestHealthCheck_NewGRPC_ReturnsSERVING NewGRPC 默认注册 health，
// 客户端 Check 应返回 SERVING（供 K8s grpc probe 使用）。
func TestHealthCheck_NewGRPC_ReturnsSERVING(t *testing.T) {
	s := NewGRPC().(*grpcServer)
	endpoint, stop := startAndWait(t, s)
	defer stop()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health Check rpc: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", resp.Status)
	}
}

// TestHealthCheck_WithoutHealth_Unimplemented WithoutHealth 关闭后，
// grpc.health.v1 未注册，Check 应返回 Unimplemented 错误。
func TestHealthCheck_WithoutHealth_Unimplemented(t *testing.T) {
	s := NewGRPC(WithoutHealth()).(*grpcServer)
	endpoint, stop := startAndWait(t, s)
	defer stop()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected Unimplemented error when health disabled, got nil")
	}
}

// TestHealthCheck_FromGRPC_WithHealth_FromGRPC 用 WithHealth 也能注册成功。
func TestHealthCheck_FromGRPC_WithHealth(t *testing.T) {
	gs := grpc.NewServer()
	s := FromGRPC(gs, WithHealth()).(*grpcServer)
	endpoint, stop := startAndWait(t, s)
	defer stop()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health Check rpc: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", resp.Status)
	}
}
