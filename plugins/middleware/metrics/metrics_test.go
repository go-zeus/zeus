package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/routing"
)

// mockMeter 用于测试，记录所有 Counter 调用
type mockMeter struct {
	counters []*mockCounter
}

func (m *mockMeter) Counter(name string, labels map[string]string) metrics.Counter {
	c := &mockCounter{name: name, labels: labels}
	m.counters = append(m.counters, c)
	return c
}
func (m *mockMeter) Histogram(_ string, _ map[string]string) metrics.Histogram { return nil }
func (m *mockMeter) Gauge(_ string, _ map[string]string) metrics.Gauge         { return nil }
func (m *mockMeter) Close() error                                              { return nil }

type mockCounter struct {
	name   string
	labels map[string]string
	inc    int
	add    float64
}

func (c *mockCounter) Inc()          { c.inc++ }
func (c *mockCounter) Add(v float64) { c.add += v }

var _ metrics.Meter = (*mockMeter)(nil)

// mockResponse 用于测试
type mockResponse struct{ status int }

func (m *mockResponse) StatusCode() int { return m.status }
func (m *mockResponse) Body() any       { return nil }

var _ middleware.Response = (*mockResponse)(nil)

type mockRequest struct{}

func (m *mockRequest) Method() string         { return "POST" }
func (m *mockRequest) Path() string           { return "/x" }
func (m *mockRequest) Header(_ string) string { return "" }
func (m *mockRequest) Body() any              { return nil }

var _ middleware.Request = (*mockRequest)(nil)

// TestIntercept_NilMeter_NoOp 验证 meter 为 nil 不报错
func TestIntercept_NilMeter_NoOp(t *testing.T) {
	i := New(nil)
	_, err := i.Intercept(context.Background(), &mockRequest{},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return &mockResponse{status: 200}, nil
		})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// TestIntercept_IncrementsCounter_WithClusterLabel 验证计数 + cluster label
func TestIntercept_IncrementsCounter_WithClusterLabel(t *testing.T) {
	meter := &mockMeter{}
	i := New(meter)

	ctx := routing.WithCluster(context.Background(), "canary")
	_, _ = i.Intercept(ctx, &mockRequest{},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return &mockResponse{status: 200}, nil
		})

	if len(meter.counters) != 1 {
		t.Fatalf("expected 1 counter, got %d", len(meter.counters))
	}
	c := meter.counters[0]
	if c.name != MetricRequestsTotal {
		t.Errorf("name = %q, want %q", c.name, MetricRequestsTotal)
	}
	if c.labels[LabelCluster] != "canary" {
		t.Errorf("cluster = %q, want canary", c.labels[LabelCluster])
	}
	if c.labels[LabelMethod] != "POST" {
		t.Errorf("method = %q, want POST", c.labels[LabelMethod])
	}
	if c.labels[LabelStatus] != "200" {
		t.Errorf("status = %q, want 200", c.labels[LabelStatus])
	}
	if c.inc != 1 {
		t.Errorf("counter inc = %d, want 1", c.inc)
	}
}

// TestIntercept_DefaultCluster_UsedAsDefaultLabel 验证 default 流量记录为 cluster=default
func TestIntercept_DefaultCluster_UsedAsDefaultLabel(t *testing.T) {
	meter := &mockMeter{}
	i := New(meter)

	_, _ = i.Intercept(context.Background(), &mockRequest{},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return &mockResponse{status: 200}, nil
		})

	if got := meter.counters[0].labels[LabelCluster]; got != routing.Default {
		t.Errorf("cluster = %q, want %q", got, routing.Default)
	}
}

// TestIntercept_ErrorStatus_500 验证错误响应记 500
func TestIntercept_ErrorStatus_500(t *testing.T) {
	meter := &mockMeter{}
	i := New(meter)

	_, _ = i.Intercept(context.Background(), &mockRequest{},
		func(ctx context.Context, _ middleware.Request) (middleware.Response, error) {
			return nil, errors.New("boom")
		})

	if got := meter.counters[0].labels[LabelStatus]; got != "500" {
		t.Errorf("status = %q, want 500", got)
	}
}

// TestName 验证中间件名
func TestName(t *testing.T) {
	i := New(nil)
	if i.Name() != "metrics" {
		t.Errorf("Name = %q, want metrics", i.Name())
	}
}
