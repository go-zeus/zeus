package grpc

import (
	"net/url"
	"testing"
)

// TestNew_RequiresTargetOrSelector 验证缺失 target 和 selector 时返回错误
func TestNew_RequiresTargetOrSelector(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("expected error when neither target nor selector is set")
	}
}

// TestNew_WithTarget 验证 WithTarget 选项正常工作
func TestNew_WithTarget(t *testing.T) {
	target, _ := url.Parse("//127.0.0.1:9090")
	p, err := New(WithTarget(target))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if p == nil {
		t.Fatal("New returned nil proxy")
	}

	// 内部状态校验
	pp := p.(*proxy)
	if pp.target == nil {
		t.Fatal("target not set")
	}
}

// TestNew_WithSelector 验证 WithSelector 选项优先级
func TestNew_WithSelector(t *testing.T) {
	s := &mockSelector{addr: "10.0.0.1:9090"}
	p, err := New(WithSelector(s))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	pp := p.(*proxy)
	if pp.selector == nil {
		t.Fatal("selector not set")
	}
}

// TestListenAndServeLifecycle 验证 Listen 但不 Serve 可正常 Stop
// 完整端到端测试需要 grpc server + client，已超出单元测试范围
func TestListenAndStop(t *testing.T) {
	target, _ := url.Parse("//127.0.0.1:9090")
	p, err := New(WithTarget(target))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	if err := p.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Listen error: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

// TestServe_NoListener 验证未 Listen 时 Serve 返回错误
func TestServe_NoListener(t *testing.T) {
	target, _ := url.Parse("//127.0.0.1:9090")
	p, err := New(WithTarget(target))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if err := p.Serve(); err == nil {
		t.Fatal("expected Serve error without Listen")
	}
}

// TestStop_Idempotent 验证多次 Stop 不出错
func TestStop_Idempotent(t *testing.T) {
	target, _ := url.Parse("//127.0.0.1:9090")
	if target == nil {
		t.Fatal("target is nil")
	}
	p, err := New(WithTarget(target))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if p == nil {
		t.Fatal("New returned nil proxy")
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

// mockSelector 测试用 selector
type mockSelector struct {
	addr string
	err  error
}

func (m *mockSelector) Pick(_ string) (*url.URL, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &url.URL{Host: m.addr}, nil
}

// 编译期检查 mockSelector 实现 selector 接口
var _ selector = (*mockSelector)(nil)
