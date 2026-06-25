package recovery

import (
	"context"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/middleware"
)

// mockRequest 模拟请求
type mockRequest struct {
	method string
	path   string
	header map[string]string
	body   any
}

func (m *mockRequest) Method() string         { return m.method }
func (m *mockRequest) Path() string           { return m.path }
func (m *mockRequest) Header(k string) string { return m.header[k] }
func (m *mockRequest) Body() any              { return m.body }

// mockResponse 模拟响应
type mockResponse struct {
	code int
	body any
}

func (m *mockResponse) StatusCode() int { return m.code }
func (m *mockResponse) Body() any       { return m.body }

func TestNew(t *testing.T) {
	d := New()
	if d.Name() != "recovery" {
		t.Errorf("Name() = %q, want %q", d.Name(), "recovery")
	}
}

func TestIntercept_NoPanic(t *testing.T) {
	d := New()
	req := &mockRequest{method: "GET", path: "/test", header: map[string]string{}}
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		return &mockResponse{code: 200, body: "ok"}, nil
	}

	resp, err := d.Intercept(context.Background(), req, handler)
	if err != nil {
		t.Fatalf("Intercept 不应返回错误，got: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("StatusCode() = %d, want 200", resp.StatusCode())
	}
}

func TestIntercept_WithPanic(t *testing.T) {
	d := New()
	req := &mockRequest{method: "POST", path: "/panic", header: map[string]string{}}
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		panic("something went wrong")
	}

	resp, err := d.Intercept(context.Background(), req, handler)
	if err == nil {
		t.Fatal("Intercept 应返回 panic 恢复错误，got nil")
	}
	if !strings.Contains(err.Error(), "panic recovered") {
		t.Errorf("错误信息应包含 'panic recovered'，got: %v", err)
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("错误信息应包含 panic 值，got: %v", err)
	}
	if !strings.Contains(err.Error(), "POST") {
		t.Errorf("错误信息应包含请求方法，got: %v", err)
	}
	if !strings.Contains(err.Error(), "/panic") {
		t.Errorf("错误信息应包含请求路径，got: %v", err)
	}
	// panic 恢复后 resp 应为 nil
	if resp != nil {
		t.Errorf("panic 恢复后 resp 应为 nil，got: %v", resp)
	}
}
