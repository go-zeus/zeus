package proxy

import (
	"context"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

// TestStaticSelector_Pick 验证静态选择器始终返回同一目标
func TestStaticSelector_Pick(t *testing.T) {
	target, _ := url.Parse("http://example.com:8080")
	s := NewStaticSelector(target)

	r := httptest.NewRequest("GET", "/foo", nil)
	got, err := s.Pick(r)
	if err != nil {
		t.Fatalf("Pick error: %v", err)
	}
	if got.String() != "http://example.com:8080" {
		t.Errorf("Pick = %v, want http://example.com:8080", got)
	}

	// 多次调用返回相同结果
	got2, _ := s.Pick(r)
	if got != got2 {
		t.Errorf("static selector should return same target, got %v and %v", got, got2)
	}
}

// TestDiscoverySelector_RoundRobin 验证动态选择器配合 roundrobin 实现轮询
func TestDiscoverySelector_RoundRobin(t *testing.T) {
	mem := memory.New()
	mem.Register(context.Background(), &types.Instance{
		ID:      "1",
		Name:    "demo",
		Cluster: "default",
		IP:      "127.0.0.1",
		Port:    9001,
	})
	mem.Register(context.Background(), &types.Instance{
		ID:      "2",
		Name:    "demo",
		Cluster: "default",
		IP:      "127.0.0.1",
		Port:    9002,
	})

	dis := mem.(interface {
		GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error)
	})

	lb := roundrobin.New()
	s := NewDiscoverySelector("demo", dis, lb)

	// 第一次选择
	r1 := httptest.NewRequest("GET", "/", nil)
	got1, err := s.Pick(r1)
	if err != nil {
		t.Fatalf("Pick 1 error: %v", err)
	}
	// 第二次选择（应轮询到下一个实例）
	got2, err := s.Pick(r1)
	if err != nil {
		t.Fatalf("Pick 2 error: %v", err)
	}

	if got1.Host == got2.Host {
		t.Errorf("roundrobin should alternate instances, got %s twice", got1.Host)
	}
}

// TestDiscoverySelector_ClusterRouting 验证集群路由命中正确 cluster
func TestDiscoverySelector_ClusterRouting(t *testing.T) {
	mem := memory.New()
	// default 集群
	mem.Register(context.Background(), &types.Instance{
		ID:      "default-1",
		Name:    "demo",
		Cluster: "default",
		IP:      "10.0.0.1",
		Port:    8080,
	})
	// canary 集群
	mem.Register(context.Background(), &types.Instance{
		ID:      "canary-1",
		Name:    "demo",
		Cluster: "canary",
		IP:      "10.0.0.2",
		Port:    8080,
	})

	dis := mem.(interface {
		GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error)
	})

	lb := roundrobin.New()
	s := NewDiscoverySelector("demo", dis, lb)

	// 无 cluster：路由到 default
	r1 := httptest.NewRequest("GET", "/", nil)
	got1, _ := s.Pick(r1)
	if got1.Host != "10.0.0.1:8080" {
		t.Errorf("default cluster should route to 10.0.0.1:8080, got %s", got1.Host)
	}

	// 指定 canary：路由到 canary
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set(routing.HeaderCluster, "canary")
	got2, _ := s.Pick(r2)
	if got2.Host != "10.0.0.2:8080" {
		t.Errorf("canary cluster should route to 10.0.0.2:8080, got %s", got2.Host)
	}
}

// TestDiscoverySelector_FallbackToDefault 验证未知 cluster 回退到 default
func TestDiscoverySelector_FallbackToDefault(t *testing.T) {
	mem := memory.New()
	mem.Register(context.Background(), &types.Instance{
		ID:      "default-1",
		Name:    "demo",
		Cluster: "default",
		IP:      "10.0.0.1",
		Port:    8080,
	})

	dis := mem.(interface {
		GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error)
	})

	s := NewDiscoverySelector("demo", dis, roundrobin.New())

	// 未知 cluster 应回退到 default
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(routing.HeaderCluster, "unknown")
	got, err := s.Pick(r)
	if err != nil {
		t.Fatalf("Pick error: %v", err)
	}
	if got.Host != "10.0.0.1:8080" {
		t.Errorf("unknown cluster should fallback to default, got %s", got.Host)
	}
}

// TestDiscoverySelector_NoCluster 验证无可用集群时返回错误
func TestDiscoverySelector_NoCluster(t *testing.T) {
	mem := memory.New()
	dis := mem.(interface {
		GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error)
	})

	s := NewDiscoverySelector("not-exist", dis, roundrobin.New())
	r := httptest.NewRequest("GET", "/", nil)
	_, err := s.Pick(r)
	if err == nil {
		t.Fatal("expected error for non-existent service")
	}
}

// TestDiscoverySelector_ConcurrentSafe 验证并发调用 Pick 不出问题
// 必须 -race 通过
func TestDiscoverySelector_ConcurrentSafe(t *testing.T) {
	mem := memory.New()
	mem.Register(context.Background(), &types.Instance{
		ID:      "1",
		Name:    "demo",
		Cluster: "default",
		IP:      "127.0.0.1",
		Port:    9001,
	})

	dis := mem.(interface {
		GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error)
	})

	s := NewDiscoverySelector("demo", dis, roundrobin.New())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("GET", "/", nil)
			_, _ = s.Pick(r)
		}()
	}
	wg.Wait()
}
