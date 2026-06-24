package gwdisc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/routing"
)

// 启动一个 mock gateway，body 由 caller 控制
func startMockGateway(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *gwapi.ServicesResponse) {
	t.Helper()
	state := &gwapi.ServicesResponse{Services: map[string][]gwapi.Instance{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/services", func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

// TestHTTPDiscovery_GetService 验证后台轮询 + 缓存返回
func TestHTTPDiscovery_GetService(t *testing.T) {
	mockSrv, _ := startMockGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(gwapi.ServicesResponse{
			Services: map[string][]gwapi.Instance{
				"srv2": {
					{ID: "srv2-d-1", Name: "srv2", Cluster: routing.Default, Protocol: "http", IP: "10.0.0.1", Port: 9002},
					{ID: "srv2-c-1", Name: "srv2", Cluster: "canary", Protocol: "http", IP: "10.0.0.2", Port: 9002},
				},
			},
		})
	})

	dis := New(mockSrv.URL)
	// 等待首次 poll 完成
	time.Sleep(100 * time.Millisecond)

	entry, err := dis.GetService(context.Background(), "srv2")
	if err != nil {
		t.Fatalf("GetService error: %v", err)
	}
	if entry == nil {
		t.Fatal("entry is nil")
	}
	if got, want := len(entry.Instances), 2; got != want {
		t.Fatalf("instance count = %d, want %d", got, want)
	}
	if got, want := len(entry.Clusters), 2; got != want {
		t.Fatalf("cluster count = %d, want %d", got, want)
	}
}

// TestHTTPDiscovery_EmptyClusterFallback 验证空 cluster 字段被规范化为 default
func TestHTTPDiscovery_EmptyClusterFallback(t *testing.T) {
	mockSrv, _ := startMockGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(gwapi.ServicesResponse{
			Services: map[string][]gwapi.Instance{
				"srv1": {
					{ID: "x", Name: "srv1", Cluster: "", Protocol: "http", IP: "1.1.1.1", Port: 9001},
				},
			},
		})
	})

	dis := New(mockSrv.URL)
	time.Sleep(100 * time.Millisecond)

	entry, _ := dis.GetService(context.Background(), "srv1")
	if _, ok := entry.Clusters[routing.Default]; !ok {
		t.Errorf("empty cluster should fallback to default")
	}
}

// TestHTTPDiscovery_NotFoundService 验证拉到空响应不报错（服务未注册场景）
func TestHTTPDiscovery_NotFoundService(t *testing.T) {
	mockSrv, _ := startMockGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(gwapi.ServicesResponse{Services: map[string][]gwapi.Instance{}})
	})

	dis := New(mockSrv.URL)
	time.Sleep(100 * time.Millisecond)

	entry, err := dis.GetService(context.Background(), "non-existent")
	if err != nil {
		t.Fatalf("GetService on non-existent should not error: %v", err)
	}
	if entry == nil {
		t.Fatal("entry should not be nil")
	}
	if len(entry.Instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(entry.Instances))
	}
}

// TestHTTPDiscovery_WatchNotifiesOnChange 验证实例变化时 watcher 收到通知
func TestHTTPDiscovery_WatchNotifiesOnChange(t *testing.T) {
	// current 在测试 goroutine 写、HTTP handler goroutine 读（json encode），
	// 必须加锁保护，否则 race detector 会报警
	var mu sync.Mutex
	current := &gwapi.ServicesResponse{Services: map[string][]gwapi.Instance{}}
	mockSrv, _ := startMockGateway(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(current)
	})

	dis := New(mockSrv.URL)
	time.Sleep(100 * time.Millisecond) // 等首次 poll

	ch, _ := dis.(*httpDiscovery).Watch(context.Background(), "srv1")

	// 触发变化：添加一个实例
	mu.Lock()
	current.Services["srv1"] = []gwapi.Instance{
		{ID: "new-1", Name: "srv1", Cluster: routing.Default, Protocol: "http", IP: "1.1.1.1", Port: 9001},
	}
	mu.Unlock()

	// 等下次 poll（间隔 2s，等待最长 3s）
	select {
	case <-ch:
		// 收到通知 ✓
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not receive notification within 3s")
	}
}
