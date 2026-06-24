package clustering

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/routing"
)

// mockRequest 模拟请求
type mockRequest struct {
	method string
	path   string
	header map[string]string
}

func (m *mockRequest) Method() string { return m.method }
func (m *mockRequest) Path() string   { return m.path }
func (m *mockRequest) Header(k string) string {
	if m.header != nil {
		return m.header[k]
	}
	return ""
}
func (m *mockRequest) Body() any { return nil }

// mockResponse 模拟响应
type mockResponse struct{}

func (m *mockResponse) StatusCode() int { return 200 }
func (m *mockResponse) Body() any       { return nil }

func TestNew(t *testing.T) {
	c := New()
	if c.Name() != "clustering" {
		t.Fatalf("expected name clustering, got %q", c.Name())
	}
}

func TestIntercept_FromHeader(t *testing.T) {
	c := New()
	var gotCluster string
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		gotCluster = routing.FromContext(ctx)
		return &mockResponse{}, nil
	}

	req := &mockRequest{
		method: "GET",
		path:   "/api",
		header: map[string]string{routing.HeaderCluster: "canary"},
	}

	resp, err := c.Intercept(context.Background(), req, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if gotCluster != "canary" {
		t.Fatalf("expected canary, got %q", gotCluster)
	}
}

func TestIntercept_FromContext(t *testing.T) {
	c := New()
	var gotCluster string
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		gotCluster = routing.FromContext(ctx)
		return &mockResponse{}, nil
	}

	ctx := routing.WithCluster(context.Background(), "gray")
	req := &mockRequest{method: "GET", path: "/api"}

	resp, err := c.Intercept(ctx, req, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if gotCluster != "gray" {
		t.Fatalf("expected gray, got %q", gotCluster)
	}
}

func TestIntercept_ContextPriority(t *testing.T) {
	// context 已有集群标记应优先于 header
	c := New()
	var gotCluster string
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		gotCluster = routing.FromContext(ctx)
		return &mockResponse{}, nil
	}

	ctx := routing.WithCluster(context.Background(), "gray")
	req := &mockRequest{
		method: "GET",
		path:   "/api",
		header: map[string]string{routing.HeaderCluster: "canary"},
	}

	_, _ = c.Intercept(ctx, req, handler)
	if gotCluster != "gray" {
		t.Fatalf("context cluster should win, expected gray, got %q", gotCluster)
	}
}

func TestIntercept_Default(t *testing.T) {
	c := New()
	var gotCluster string
	handler := func(ctx context.Context, req middleware.Request) (middleware.Response, error) {
		gotCluster = routing.FromContext(ctx)
		return &mockResponse{}, nil
	}

	req := &mockRequest{method: "GET", path: "/api"}
	_, _ = c.Intercept(context.Background(), req, handler)
	if gotCluster != routing.Default {
		t.Fatalf("expected default, got %q", gotCluster)
	}
}

func TestClusterFromHTTP(t *testing.T) {
	r := httptest.NewRequest("GET", "/api", nil)
	r.Header.Set(routing.HeaderCluster, "canary")
	ctx := ClusterFromHTTP(r)

	cluster := routing.FromContext(ctx)
	if cluster != "canary" {
		t.Fatalf("expected canary, got %q", cluster)
	}
}

func TestClusterFromHTTP_Default(t *testing.T) {
	r := httptest.NewRequest("GET", "/api", nil)
	ctx := ClusterFromHTTP(r)

	cluster := routing.FromContext(ctx)
	if cluster != routing.Default {
		t.Fatalf("expected default, got %q", cluster)
	}
}
