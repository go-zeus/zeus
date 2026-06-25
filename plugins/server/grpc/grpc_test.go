package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-zeus/zeus/routing"
	"google.golang.org/grpc"
	grpcmeta "google.golang.org/grpc/metadata"
)

// TestClusterInterceptor_FromMetadata 验证从 metadata 提取 cluster
func TestClusterInterceptor_FromMetadata(t *testing.T) {
	ctx := grpcmeta.NewIncomingContext(context.Background(), grpcmeta.Pairs(routing.MetadataCluster, "canary"))
	var gotCluster string
	_, _ = clusterInterceptor(ctx, nil, nil, func(ctx context.Context, _ interface{}) (interface{}, error) {
		gotCluster = routing.FromContext(ctx)
		return nil, nil
	})
	if gotCluster != "canary" {
		t.Fatalf("got %q, want canary", gotCluster)
	}
}

// TestClusterInterceptor_MissingMetadata_FallsBackToDefault 验证缺失时回退 default
func TestClusterInterceptor_MissingMetadata_FallsBackToDefault(t *testing.T) {
	var gotCluster string
	_, _ = clusterInterceptor(context.Background(), nil, nil, func(ctx context.Context, _ interface{}) (interface{}, error) {
		gotCluster = routing.FromContext(ctx)
		return nil, nil
	})
	if gotCluster != routing.Default {
		t.Fatalf("got %q, want %q", gotCluster, routing.Default)
	}
}

// TestNewGRPC_DefaultOptions 验证默认端口和 autoClustering
func TestNewGRPC_DefaultOptions(t *testing.T) {
	s := NewGRPC().(*grpcServer)
	if s.port != DefaultPort {
		t.Errorf("port = %d, want %d", s.port, DefaultPort)
	}
	if !s.autoClustering {
		t.Error("autoClustering should default to true")
	}
}

// TestNewGRPC_OptionsApply 验证 Option 生效
func TestNewGRPC_OptionsApply(t *testing.T) {
	s := NewGRPC(Port(1234), Ip("127.0.0.1"), WithoutAutoClustering()).(*grpcServer)
	if s.port != 1234 {
		t.Errorf("port = %d, want 1234", s.port)
	}
	if s.ip != "127.0.0.1" {
		t.Errorf("ip = %q, want 127.0.0.1", s.ip)
	}
	if s.autoClustering {
		t.Error("autoClustering should be false")
	}
	if s.Endpoint() != "127.0.0.1:1234" {
		t.Errorf("Endpoint = %q, want 127.0.0.1:1234", s.Endpoint())
	}
}

// TestRegister_AppendOrder 验证 Register 多次调用按顺序追加
func TestRegister_AppendOrder(t *testing.T) {
	var order []int
	s := NewGRPC(
		Register(func(*grpc.Server) { order = append(order, 1) }),
		Register(func(*grpc.Server) { order = append(order, 2) }),
		Register(func(*grpc.Server) { order = append(order, 3) }),
	).(*grpcServer)
	for _, r := range s.registers {
		r(nil)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("order = %v, want [1 2 3]", order)
	}
}

// TestStart_ListenAndServe_ListenFails 验证监听失败端口正确报错
func TestStart_ListenFails(t *testing.T) {
	// 取一个已知被占用端口（先 listen 一次）
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().(*net.TCPAddr)
	defer ln.Close()

	s := NewGRPC(Port(addr.Port), Ip("127.0.0.1"))
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected listen error")
	}
}

// TestStart_GracefulShutdown 验证 ctx 取消触发 GracefulStop
func TestStart_GracefulShutdown(t *testing.T) {
	// 取一个随机端口（先 listen 再关闭腾出端口）
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().(*net.TCPAddr)
	_ = ln.Close()

	s := NewGRPC(Ip("127.0.0.1"), Port(addr.Port))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Start(ctx)
		close(done)
	}()

	// 给 server 一点时间启动
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

// TestStop_Idempotent 验证多次 Stop 不出错
func TestStop_Idempotent(t *testing.T) {
	s := NewGRPC()
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
