package registry_test

import (
	"context"
	"testing"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/types"
)

// 验证 memory.New() 满足 registry.Registrar 接口
func TestMemoryImplementsRegistrar(t *testing.T) {
	var _ registry.Registrar = memory.New()
}

// 验证 memory.New() 返回值同时满足 registry.Discovery 和 registry.Watcher 接口
func TestMemoryImplementsDiscoveryAndWatcher(t *testing.T) {
	m := memory.New()
	if _, ok := m.(registry.Discovery); !ok {
		t.Error("memory.New() should implement registry.Discovery")
	}
	if _, ok := m.(registry.Watcher); !ok {
		t.Error("memory.New() should implement registry.Watcher")
	}
}

func TestMemoryRegisterAndGetService(t *testing.T) {
	m := memory.New()
	ins := &types.Instance{Name: "test-svc", IP: "127.0.0.1", Port: 8080}

	if err := m.Register(context.Background(), ins); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 类型断言获取 Discovery
	dis := m.(registry.Discovery)
	svc, err := dis.GetService(context.Background(), "test-svc")
	if err != nil {
		t.Fatalf("get service failed: %v", err)
	}
	if svc == nil {
		t.Fatal("expected service")
	}
}

func TestMemoryDeregister(t *testing.T) {
	m := memory.New()
	ins := &types.Instance{Name: "test-svc", IP: "127.0.0.1", Port: 8080}

	m.Register(context.Background(), ins)
	if err := m.Deregister(context.Background(), ins); err != nil {
		t.Fatalf("deregister failed: %v", err)
	}
}

func TestMemoryDeregister_NotExist(t *testing.T) {
	m := memory.New()
	ins := &types.Instance{Name: "not-exist", IP: "127.0.0.1", Port: 8080}
	// 幂等语义：注销不存在的服务返回 nil（对齐 K8s delete）
	if err := m.Deregister(context.Background(), ins); err != nil {
		t.Fatalf("deregister non-existent service should be idempotent, got: %v", err)
	}
}

func TestMemoryWatch(t *testing.T) {
	m := memory.New()
	watcher := m.(registry.Watcher)
	ch, err := watcher.Watch(context.Background(), "test-svc")
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	if ch == nil {
		t.Fatal("expected channel")
	}

	// 注册后应触发 watcher
	ins := &types.Instance{Name: "test-svc", IP: "127.0.0.1", Port: 8080}
	m.Register(context.Background(), ins)

	select {
	case <-ch:
		// 正常收到通知
	default:
		t.Fatal("expected watcher notification")
	}
}
