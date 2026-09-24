package gwreg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
)

// fakeGateway 模拟 gateway 的 /internal/register 端点
// restart() 清空已注册实例，模拟 gateway 重启后 memory registry 状态丢失
type fakeGateway struct {
	mu       sync.Mutex
	register map[string]bool // id → 已注册
	postCnt  int
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{register: make(map[string]bool)}
}

func (f *fakeGateway) handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req gwapi.RegisterRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.register[req.Instance.ID] = true
		f.postCnt++
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(gwapi.RegisterResponse{OK: true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeGateway) has(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.register[id]
}

// restart 清空注册表（memory registry 状态随进程丢失）
func (f *fakeGateway) restart() {
	f.mu.Lock()
	f.register = make(map[string]bool)
	f.mu.Unlock()
}

func testInstance() gwapi.Instance {
	return gwapi.Instance{ID: "srv1-test", Name: "srv1", Cluster: "default", Protocol: "http", IP: "127.0.0.1", Port: 9001}
}

// TestKeepAlive_ReRegistersAfterGatewayRestart 验证核心场景：
// gateway 重启导致注册丢失后，心跳在下一个周期自动恢复注册
func TestKeepAlive_ReRegistersAfterGatewayRestart(t *testing.T) {
	old := keepAliveInterval
	keepAliveInterval = 30 * time.Millisecond
	defer func() { keepAliveInterval = old }()

	gw := newFakeGateway()
	srv := httptest.NewServer(http.HandlerFunc(gw.handler))
	defer srv.Close()

	c := New(srv.URL)
	ins := testInstance()
	if err := c.Register(context.Background(), ins); err != nil {
		t.Fatalf("initial register: %v", err)
	}
	if !gw.has(ins.ID) {
		t.Fatal("initial register should succeed")
	}

	stop := c.KeepAlive(ins)
	defer stop()

	// 模拟 gateway 重启：注册表清空
	gw.restart()
	if gw.has(ins.ID) {
		t.Fatal("restart should clear registry")
	}

	// 等待超过一个心跳周期，断言自动恢复
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if gw.has(ins.ID) {
			return // 恢复成功
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("keepalive should re-register after gateway restart")
}

// TestKeepAlive_StopHalts 验证 stop() 后不再心跳（优雅关闭语义）
func TestKeepAlive_StopHalts(t *testing.T) {
	old := keepAliveInterval
	keepAliveInterval = 30 * time.Millisecond
	defer func() { keepAliveInterval = old }()

	gw := newFakeGateway()
	srv := httptest.NewServer(http.HandlerFunc(gw.handler))
	defer srv.Close()

	c := New(srv.URL)
	ins := testInstance()
	if err := c.Register(context.Background(), ins); err != nil {
		t.Fatalf("initial register: %v", err)
	}

	stop := c.KeepAlive(ins)
	stop() // 立即停止

	gw.restart()
	gw.mu.Lock()
	cntAfterStop := gw.postCnt
	gw.mu.Unlock()

	time.Sleep(100 * time.Millisecond) // 超过 3 个心跳周期

	gw.mu.Lock()
	postsAfterStop := gw.postCnt
	stillRegistered := gw.register[ins.ID]
	gw.mu.Unlock()
	if postsAfterStop != cntAfterStop {
		t.Fatalf("stop() should halt keepalive, got %d extra posts", postsAfterStop-cntAfterStop)
	}
	if stillRegistered {
		t.Fatal("no re-register should happen after stop()")
	}
}
