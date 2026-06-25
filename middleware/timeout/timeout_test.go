package timeout

import (
	"context"
	"strings"
	"testing"
	"time"

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
	d := New(30 * time.Second)
	if d.Name() != "timeout" {
		t.Errorf("Name() = %q, want %q", d.Name(), "timeout")
	}
}

func TestIntercept_NoTimeout(t *testing.T) {
	d := New(100 * time.Millisecond)
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

func TestIntercept_TimeoutExceeded(t *testing.T) {
	d := New(100 * time.Millisecond)
	req := &mockRequest{method: "GET", path: "/slow", header: map[string]string{}}
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		time.Sleep(200 * time.Millisecond)
		return &mockResponse{code: 200, body: "ok"}, nil
	}

	_, err := d.Intercept(context.Background(), req, handler)
	if err == nil {
		t.Fatal("超时后应返回错误，got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("错误信息应包含 'timed out'，got: %v", err)
	}
}
