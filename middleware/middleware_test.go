package middleware

import (
	"context"
	"testing"
)

// ---- mock implementations ----

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

type mockResponse struct {
	code int
	body any
}

func (m *mockResponse) StatusCode() int { return m.code }
func (m *mockResponse) Body() any       { return m.body }

type mockInterceptor struct {
	name string
	// interceptFn 如果不为 nil 则调用，否则直接执行 handler
	interceptFn func(ctx context.Context, req Request, handler Handler) (Response, error)
}

func (m *mockInterceptor) Intercept(ctx context.Context, req Request, handler Handler) (Response, error) {
	if m.interceptFn != nil {
		return m.interceptFn(ctx, req, handler)
	}
	return handler(ctx, req)
}

func (m *mockInterceptor) Name() string { return m.name }

// ---- tests ----

func TestChain(t *testing.T) {
	var order []string

	mw1 := &mockInterceptor{
		name: "first",
		interceptFn: func(ctx context.Context, req Request, handler Handler) (Response, error) {
			order = append(order, "mw1-before")
			resp, err := handler(ctx, req)
			order = append(order, "mw1-after")
			return resp, err
		},
	}

	mw2 := &mockInterceptor{
		name: "second",
		interceptFn: func(ctx context.Context, req Request, handler Handler) (Response, error) {
			order = append(order, "mw2-before")
			resp, err := handler(ctx, req)
			order = append(order, "mw2-after")
			return resp, err
		},
	}

	chain := NewChain(mw1, mw2)

	final := func(ctx context.Context, req Request) (Response, error) {
		order = append(order, "final")
		return &mockResponse{code: 200, body: "ok"}, nil
	}

	resp, err := chain.Handle(context.Background(), &mockRequest{method: "GET", path: "/test"}, final)
	if err != nil {
		t.Fatalf("Chain.Handle error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("StatusCode() = %d, want 200", resp.StatusCode())
	}

	// 验证中间件链执行顺序：mw1 -> mw2 -> final -> mw2-after -> mw1-after
	expected := []string{"mw1-before", "mw2-before", "final", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("execution order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestChainEmpty(t *testing.T) {
	chain := NewChain()

	called := false
	final := func(ctx context.Context, req Request) (Response, error) {
		called = true
		return &mockResponse{code: 200, body: "ok"}, nil
	}

	resp, err := chain.Handle(context.Background(), &mockRequest{method: "GET", path: "/"}, final)
	if err != nil {
		t.Fatalf("Chain.Handle error: %v", err)
	}
	if !called {
		t.Error("final handler was not called")
	}
	if resp.StatusCode() != 200 {
		t.Errorf("StatusCode() = %d, want 200", resp.StatusCode())
	}
}

// ---- 新增测试 ----

// TestChain_Append 验证 Chain.Append 追加中间件
func TestChain_Append(t *testing.T) {
	var order []string

	mw1 := &mockInterceptor{
		name: "first",
		interceptFn: func(ctx context.Context, req Request, handler Handler) (Response, error) {
			order = append(order, "mw1")
			return handler(ctx, req)
		},
	}

	mw2 := &mockInterceptor{
		name: "second",
		interceptFn: func(ctx context.Context, req Request, handler Handler) (Response, error) {
			order = append(order, "mw2")
			return handler(ctx, req)
		},
	}

	chain := NewChain(mw1)
	chain = chain.Append(mw2)

	final := func(ctx context.Context, req Request) (Response, error) {
		order = append(order, "final")
		return &mockResponse{code: 200, body: "ok"}, nil
	}

	_, err := chain.Handle(context.Background(), &mockRequest{method: "GET", path: "/"}, final)
	if err != nil {
		t.Fatalf("Chain.Handle error: %v", err)
	}

	expected := []string{"mw1", "mw2", "final"}
	if len(order) != len(expected) {
		t.Fatalf("execution order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

// TestInterceptor_Interface 验证 Interceptor 接口方法可调用
func TestInterceptor_Interface(t *testing.T) {
	called := false
	ic := &mockInterceptor{
		name: "intercept_test",
		interceptFn: func(ctx context.Context, req Request, handler Handler) (Response, error) {
			called = true
			return handler(ctx, req)
		},
	}

	final := func(ctx context.Context, req Request) (Response, error) {
		return &mockResponse{code: 200}, nil
	}

	resp, err := ic.Intercept(context.Background(), &mockRequest{method: "GET", path: "/"}, final)
	if err != nil {
		t.Fatalf("Intercept error: %v", err)
	}
	if !called {
		t.Error("Interceptor Intercept was not called")
	}
	if resp.StatusCode() != 200 {
		t.Errorf("StatusCode() = %d, want 200", resp.StatusCode())
	}
}
