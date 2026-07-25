package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/types"
)

// 验证 New() 返回值满足 registry.Registrar 接口
func TestNewImplementsRegistrar(t *testing.T) {
	var _ registry.Registrar = New()
}

// 验证 New() 返回值同时满足 registry.Discovery 和 registry.Watcher 接口
func TestNewImplementsDiscoveryAndWatcher(t *testing.T) {
	m := New()
	if _, ok := m.(registry.Discovery); !ok {
		t.Error("New() should implement registry.Discovery")
	}
	if _, ok := m.(registry.Watcher); !ok {
		t.Error("New() should implement registry.Watcher")
	}
}

func TestNewMemory(t *testing.T) {
	m := NewMemory()
	if m == nil {
		t.Fatal("expected non-nil registrar")
	}
}

func TestRegisterAndGetService(t *testing.T) {
	m := NewMemory().(*memory)
	ins := &types.Instance{Name: "test-svc", IP: "127.0.0.1", Port: 8080}

	if err := m.Register(context.Background(), ins); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	svc, err := m.GetService(context.Background(), "test-svc")
	if err != nil {
		t.Fatalf("get service failed: %v", err)
	}
	if svc == nil {
		t.Fatal("expected service")
	}
}

func TestDeregister(t *testing.T) {
	m := NewMemory().(*memory)
	ins := &types.Instance{Name: "test-svc", IP: "127.0.0.1", Port: 8080}

	m.Register(context.Background(), ins)
	if err := m.Deregister(context.Background(), ins); err != nil {
		t.Fatalf("deregister failed: %v", err)
	}
}

// TestDeregister_NotExist 注销不存在服务时应幂等返回 nil（对齐 K8s delete 语义）
// 避免调用方在重复注销或竞态场景下需要额外处理错误
func TestDeregister_NotExist(t *testing.T) {
	m := NewMemory().(*memory)
	ins := &types.Instance{Name: "not-exist", IP: "127.0.0.1", Port: 8080}
	if err := m.Deregister(context.Background(), ins); err != nil {
		t.Fatalf("deregister non-existent service should be idempotent, got: %v", err)
	}
}

// TestDeregister_DuplicateInstance 重复注销已注销实例应幂等成功
func TestDeregister_DuplicateInstance(t *testing.T) {
	m := NewMemory().(*memory)
	ins := &types.Instance{ID: "ins-1", Name: "svc", Cluster: "default", IP: "127.0.0.1", Port: 8080}
	_ = m.Register(context.Background(), ins)
	if err := m.Deregister(context.Background(), ins); err != nil {
		t.Fatalf("first deregister: %v", err)
	}
	if err := m.Deregister(context.Background(), ins); err != nil {
		t.Fatalf("second deregister should be idempotent, got: %v", err)
	}
}

// TestRegister_DuplicateInstance 重复注册相同 id 应返回 error
func TestRegister_DuplicateInstance(t *testing.T) {
	m := NewMemory().(*memory)
	ins := &types.Instance{ID: "ins-1", Name: "svc", Cluster: "default", IP: "127.0.0.1", Port: 8080}
	if err := m.Register(context.Background(), ins); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := m.Register(context.Background(), ins); err == nil {
		t.Fatal("duplicate register should return error")
	}
}

func TestWatch(t *testing.T) {
	m := NewMemory().(*memory)
	ch, err := m.Watch(context.Background(), "test-svc")
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

func TestConcurrentAccess(t *testing.T) {
	m := NewMemory().(*memory)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ins := &types.Instance{Name: "svc", IP: "127.0.0.1", Port: 8080 + i}
			m.Register(context.Background(), ins)
		}(i)
	}
	wg.Wait()

	svc, _ := m.GetService(context.Background(), "svc")
	if svc == nil {
		t.Fatal("expected service after concurrent registers")
	}
}

// TestGetService_ConcurrentIterationSafe 回归测试：
// GetService 返回快照后，调用方无锁 range Clusters/Instances 必须与并发的
// Register/Deregister 互不干扰。修复前直接返回内部 *ServiceEntry 指针，
// 此场景会触发 "concurrent map iteration and map write" panic（-race 下必现）。
func TestGetService_ConcurrentIterationSafe(t *testing.T) {
	m := NewMemory().(*memory)
	// 预置初始实例（分散到多个 cluster，使 Clusters map 非空）
	for i := 0; i < 4; i++ {
		cluster := "default"
		if i%2 == 1 {
			cluster = "canary"
		}
		_ = m.Register(context.Background(), &types.Instance{
			ID: fmt.Sprintf("ins-%d", i), Name: "svc", Cluster: cluster,
			IP: "127.0.0.1", Port: 9000 + i,
		})
	}

	var wg sync.WaitGroup
	const writers = 8
	const readers = 8

	// 写者：不断 Register/Deregister 实例，持续修改内部 Clusters map
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				cluster := "default"
				if i%3 == 0 {
					cluster = "canary"
				}
				ins := &types.Instance{
					ID: fmt.Sprintf("w-%d-%d", w, i), Name: "svc", Cluster: cluster,
					IP: "10.0.0.1", Port: 10000 + (i % 1000),
				}
				_ = m.Register(context.Background(), ins)
				m.Deregister(context.Background(), ins)
			}
		}(w)
	}

	// 读者：GetService 拿快照后无锁遍历 Clusters 与 Instances
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				svc, err := m.GetService(context.Background(), "svc")
				if err != nil || svc == nil {
					continue
				}
				// 无锁遍历快照的 map（修复前会 panic）
				for _, cl := range svc.Clusters {
					_ = cl.GetInstances()
				}
				for range svc.Instances {
				}
			}
		}()
	}

	wg.Wait()
}

// TestClose_ClosesAllWatchers Close 后所有 watcher channel 应被关闭
func TestClose_ClosesAllWatchers(t *testing.T) {
	m := NewMemory().(*memory)
	ch1, _ := m.Watch(context.Background(), "svc")
	ch2, _ := m.Watch(context.Background(), "svc")

	m.Close()

	// 两个 channel 都应被关闭（读取返回 zero value + ok=false）
	if _, ok := <-ch1; ok {
		t.Error("ch1 should be closed after Close()")
	}
	if _, ok := <-ch2; ok {
		t.Error("ch2 should be closed after Close()")
	}
}

// TestClose_Idempotent Close 多次调用不 panic
func TestClose_Idempotent(t *testing.T) {
	m := NewMemory().(*memory)
	m.Close()
	// 第二次 Close 不应 panic 或阻塞
	m.Close()
}

// TestWatch_AfterClosed Close 后再 Watch 返回已关闭的 channel
func TestWatch_AfterClosed(t *testing.T) {
	m := NewMemory().(*memory)
	m.Close()
	ch, err := m.Watch(context.Background(), "svc")
	if err != nil {
		t.Fatalf("Watch after Close should not return error, got: %v", err)
	}
	if _, ok := <-ch; ok {
		t.Error("channel should be already closed")
	}
}

// TestRegister_AfterClose Close 后注册仍可成功（memory 不拒绝写入，仅影响 watcher）
func TestRegister_AfterClose(t *testing.T) {
	m := NewMemory().(*memory)
	m.Close()
	ins := &types.Instance{Name: "svc", IP: "127.0.0.1", Port: 8080}
	if err := m.Register(context.Background(), ins); err != nil {
		t.Fatalf("Register after Close should still succeed, got: %v", err)
	}
}

// TestNotifyWatchers_DroppedEvent watcher 不消费时事件被丢弃（非阻塞写入）
func TestNotifyWatchers_DroppedEvent(t *testing.T) {
	m := NewMemory().(*memory)
	ch, _ := m.Watch(context.Background(), "svc")
	ins := &types.Instance{Name: "svc", IP: "127.0.0.1", Port: 8080}

	// 连续触发多次通知，watcher 未消费 → 应不阻塞
	for i := 0; i < 10; i++ {
		_ = m.Register(context.Background(), &types.Instance{
			ID:   "ins-" + string(rune('A'+i)),
			Name: "svc", IP: "127.0.0.1", Port: 9000 + i,
		})
	}
	// 仅消费一次即可（最新事件可见）
	select {
	case <-ch:
	default:
		t.Error("expected at least one notification event")
	}
	_ = ins
}
