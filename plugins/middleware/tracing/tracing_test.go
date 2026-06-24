package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/trace"
)

// mockTracer 用于测试，记录 span 创建/结束及 attribute
type mockTracer struct {
	spans []*mockSpan
}

func (t *mockTracer) StartSpan(_ context.Context, name string, _ ...trace.SpanOption) (context.Context, trace.Span) {
	s := &mockSpan{name: name, attrs: map[string]string{}}
	t.spans = append(t.spans, s)
	return context.Background(), s
}

func (t *mockTracer) Close() error { return nil }

type mockSpan struct {
	name      string
	attrs     map[string]string
	ended     bool
	err       error
	recording bool
}

func (s *mockSpan) End() { s.ended = true }
func (s *mockSpan) SetAttributes(attrs map[string]string) {
	for k, v := range attrs {
		s.attrs[k] = v
	}
}
func (s *mockSpan) SetName(name string)   { s.name = name }
func (s *mockSpan) RecordError(err error) { s.err = err }
func (s *mockSpan) IsRecording() bool     { return s.recording }

// 编译期检查 mockTracer 实现 trace.Tracer
var _ trace.Tracer = (*mockTracer)(nil)

// mockRequest 用于测试
type mockRequest struct {
	method string
	path   string
}

func (m *mockRequest) Method() string         { return m.method }
func (m *mockRequest) Path() string           { return m.path }
func (m *mockRequest) Header(_ string) string { return "" }
func (m *mockRequest) Body() any              { return nil }

var _ middleware.Request = (*mockRequest)(nil)

// TestNew_NilTracer_NoOp 验证 tracer 为 nil 时不创建 span
func TestNew_NilTracer_NoOp(t *testing.T) {
	i := New(nil)
	called := false
	resp, err := i.Intercept(context.Background(), &mockRequest{method: "GET", path: "/x"},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			called = true
			return nil, nil
		})
	if !called {
		t.Error("handler should be called")
	}
	if resp != nil || err != nil {
		t.Errorf("unexpected resp/err: %v %v", resp, err)
	}
}

// TestIntercept_CreatesSpan_WithClusterAttribute 验证 cluster 注入 span attribute
func TestIntercept_CreatesSpan_WithClusterAttribute(t *testing.T) {
	tracer := &mockTracer{}
	i := New(tracer)

	ctx := routing.WithCluster(context.Background(), "canary")
	_, _ = i.Intercept(ctx, &mockRequest{method: "GET", path: "/ping"},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return nil, nil
		})

	if len(tracer.spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(tracer.spans))
	}
	s := tracer.spans[0]
	if s.name != "GET /ping" {
		t.Errorf("span name = %q, want GET /ping", s.name)
	}
	if !s.ended {
		t.Error("span should be ended")
	}
	if got := s.attrs[AttributeCluster]; got != "canary" {
		t.Errorf("attr %s = %q, want canary", AttributeCluster, got)
	}
}

// TestIntercept_DefaultCluster_NotRecorded 验证 default cluster 不污染 span
func TestIntercept_DefaultCluster_NotRecorded(t *testing.T) {
	tracer := &mockTracer{}
	i := New(tracer)

	_, _ = i.Intercept(context.Background(), &mockRequest{method: "GET", path: "/x"},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return nil, nil
		})

	if _, ok := tracer.spans[0].attrs[AttributeCluster]; ok {
		t.Error("default cluster should not be recorded in span attributes")
	}
}

// TestIntercept_HandlerError_Propagated 验证 handler 错误正常传递
func TestIntercept_HandlerError_Propagated(t *testing.T) {
	tracer := &mockTracer{}
	i := New(tracer)

	want := errors.New("boom")
	_, err := i.Intercept(context.Background(), &mockRequest{method: "GET", path: "/x"},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return nil, want
		})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

// TestName 验证中间件名
func TestName(t *testing.T) {
	i := New(nil)
	if i.Name() != "tracing" {
		t.Errorf("Name() = %q, want tracing", i.Name())
	}
}
