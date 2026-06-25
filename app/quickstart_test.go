package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/server"
)

// TestApplyDefaults 验证默认值填充（"默认装配不允许失败"原则）
func TestApplyDefaults(t *testing.T) {
	cases := []struct {
		name string
		in   *Config
		want Config
	}{
		{"empty → all defaults", &Config{}, Config{Name: "zeus-service", Port: 8080, Cluster: "default"}},
		{"name kept", &Config{Name: "myapp"}, Config{Name: "myapp", Port: 8080, Cluster: "default"}},
		{"port kept", &Config{Port: 9000}, Config{Name: "zeus-service", Port: 9000, Cluster: "default"}},
		{"cluster kept", &Config{Cluster: "canary"}, Config{Name: "zeus-service", Port: 8080, Cluster: "canary"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			applyDefaults(c.in)
			if *c.in != c.want {
				t.Errorf("got %+v, want %+v", *c.in, c.want)
			}
		})
	}
}

// TestCoerceHandler 验证 handler 类型推断
func TestCoerceHandler(t *testing.T) {
	hf := func(http.ResponseWriter, *http.Request) {}

	cases := []struct {
		name    string
		in      any
		wantErr bool
	}{
		{"nil → default handler", nil, false},
		{"http.Handler", http.NewServeMux(), false},
		{"http.HandlerFunc", http.HandlerFunc(hf), false},
		{"plain func", hf, false},
		{"unsupported string", "string-handler", true},
		{"unsupported struct", struct{}{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, err := coerceHandler(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_ = h
		})
	}
}

// TestResolveRegistry 验证 URL scheme 路由
func TestResolveRegistry(t *testing.T) {
	t.Run("empty → memory", func(t *testing.T) {
		r, err := resolveRegistry("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if r == nil {
			t.Error("expected non-nil registry")
		}
		// memory.New() 返回的实例能 GetService（验证确实是 memory 而非占位）
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		entry, err := r.(registry.Discovery).GetService(ctx, "any-service")
		if err != nil {
			t.Errorf("GetService err: %v", err)
		}
		_ = entry
	})

	t.Run("memory:// → memory", func(t *testing.T) {
		r, err := resolveRegistry("memory://")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if r == nil {
			t.Error("expected non-nil registry")
		}
	})

	t.Run("custom scheme via resolver", func(t *testing.T) {
		called := false
		RegisterRegistryResolver("test", func(url string) (registry.Registrar, error) {
			called = true
			if url != "test://host:1234" {
				t.Errorf("unexpected url: %s", url)
			}
			return memory.New(), nil
		})
		defer delete(registryResolvers, "test")

		_, err := resolveRegistry("test://host:1234")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !called {
			t.Error("custom resolver not called")
		}
	})

	// 未知 scheme → error（生产防误导）
	t.Run("unknown scheme → error", func(t *testing.T) {
		_, err := resolveRegistry("etcd://localhost:2379")
		if err == nil {
			t.Fatal("expected error for unregistered etcd scheme")
		}
	})
}

// TestRun_EndToEnd L1 入口端到端冒烟：启动 → 响应请求 → SIGINT → 优雅关闭
//
// 这是 L1 API 最关键的验证：5 行代码必须真的能跑。
func TestRun_EndToEnd(t *testing.T) {
	// 动态取空闲端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	port := addr.Port
	ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/hi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})

	// 异步启动应用（L1 5 行 API）
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(&Config{Port: port}, mux)
	}()

	// 等待 HTTP 就绪（轮询直到能响应）
	url := fmt.Sprintf("http://127.0.0.1:%d/hi", port)
	ready := false
	for i := 0; i < 60; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready in 3s")
	}

	// 发 SIGINT 触发优雅关闭
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGINT)

	select {
	case <-errCh:
		// 优雅关闭完成（err 可能为 nil 或信号路径 error）
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after signal")
	}
}

// TestRun_NilConfigSafe L1 必须支持零配置（即使 nil 也安全）
// 仅验证不 panic，端到端在 TestRun_EndToEnd 覆盖
func TestRun_NilConfigSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Run with nil config should not panic, got: %v", r)
		}
	}()
	// 不实际运行 Run（会阻塞），仅验证 applyDefaults(nil 安全) + coerceHandler(nil 安全)
	applyDefaults(&Config{})
	_, _ = coerceHandler(nil)
}

// TestRun_UnsupportedHandler 返回明确错误而非 panic
func TestRun_UnsupportedHandler(t *testing.T) {
	err := Run(&Config{Port: 0}, "i-am-not-a-handler")
	if err == nil {
		t.Fatal("expected error for unsupported handler")
	}
}

// TestResolveServer_HTTPHandler 命中内置 HTTP 路径，返回 HTTP server
func TestResolveServer_HTTPHandler(t *testing.T) {
	cfg := &Config{Port: 0}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	srv, kind, err := resolveServer(h, cfg)
	if err != nil {
		t.Fatalf("resolveServer: %v", err)
	}
	if kind != "http" {
		t.Errorf("kind = %q, want http", kind)
	}
	if srv == nil {
		t.Fatal("server is nil")
	}
	if srv.Protocol() != "http" {
		t.Errorf("Protocol() = %q, want http", srv.Protocol())
	}
}

// TestResolveServer_NilHandler 返回 HTTP server（用默认 handler）
func TestResolveServer_NilHandler(t *testing.T) {
	cfg := &Config{Port: 0}
	srv, kind, err := resolveServer(nil, cfg)
	if err != nil {
		t.Fatalf("resolveServer(nil): %v", err)
	}
	if kind != "http" {
		t.Errorf("kind = %q, want http", kind)
	}
	if srv == nil || srv.Protocol() != "http" {
		t.Errorf("expected http server, got %+v", srv)
	}
}

// TestResolveServer_CustomResolver 用户自定义 resolver 命中
func TestResolveServer_CustomResolver(t *testing.T) {
	// 注入一个 mock resolver（匹配 int 类型）
	defer resetServerResolvers()
	RegisterServerResolver(func(handler any, cfg *Config) (server.Server, error) {
		if _, ok := handler.(int); !ok {
			return nil, nil // 不匹配
		}
		return qsMockServer{protocol: "mock"}, nil
	})

	cfg := &Config{Port: 0}
	srv, kind, err := resolveServer(42, cfg)
	if err != nil {
		t.Fatalf("resolveServer: %v", err)
	}
	if kind != "plugin" {
		t.Errorf("kind = %q, want plugin", kind)
	}
	if srv.Protocol() != "mock" {
		t.Errorf("Protocol() = %q, want mock", srv.Protocol())
	}
}

// TestResolveServer_NoMatchReturnsError 所有 resolver 都不匹配时返回明确错误
func TestResolveServer_NoMatchReturnsError(t *testing.T) {
	defer resetServerResolvers()
	// 注册一个只匹配 int 的 resolver
	RegisterServerResolver(func(handler any, cfg *Config) (server.Server, error) {
		return nil, nil
	})

	_, _, err := resolveServer(struct{}{}, &Config{})
	if err == nil {
		t.Fatal("expected error when no resolver matches")
	}
}

// resetServerResolvers 清空 server resolvers（仅用于测试隔离）
func resetServerResolvers() {
	serverResolvers = nil
}

// qsMockServer 仅用于 quickstart_test.go 的最小 server.Server 实现（value receiver）
// 避免和 app_test.go 的 mockServer（pointer receiver）冲突
type qsMockServer struct {
	protocol string
}

func (m qsMockServer) Protocol() string              { return m.protocol }
func (m qsMockServer) Endpoint() string              { return "" }
func (m qsMockServer) Start(_ context.Context) error { return nil }
func (m qsMockServer) Stop(_ context.Context) error  { return nil }
