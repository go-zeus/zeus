package trace_test

import (
	"context"
	"testing"

	"github.com/go-zeus/zeus/trace"
	"github.com/go-zeus/zeus/trace/noop"
)

func TestNoopNew(t *testing.T) {
	tr := noop.New()
	if tr == nil {
		t.Fatal("expected non-nil Tracer")
	}
}

func TestNoopStartSpan(t *testing.T) {
	tr := noop.New()
	ctx, span := tr.StartSpan(context.Background(), "test")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestNoopSpanOperations(t *testing.T) {
	tr := noop.New()
	_, span := tr.StartSpan(context.Background(), "test")
	span.SetAttributes(map[string]string{"key": "value"})
	span.SetName("renamed")
	span.RecordError(nil)
	if span.IsRecording() {
		t.Fatal("noop span should not be recording")
	}
	span.End()
}

func TestNoopClose(t *testing.T) {
	tr := noop.New()
	if err := tr.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTracerInterface(t *testing.T) {
	// 验证 noop.New() 返回值满足 trace.Tracer 接口
	var _ trace.Tracer = noop.New()
}

func TestSpanOption(t *testing.T) {
	// 验证 SpanOption 和 SpanConfig 可正常使用
	opt := trace.SpanOption(func(cfg *trace.SpanConfig) {
		cfg.Kind = trace.SpanKindServer
		cfg.Attrs = map[string]string{"key": "val"}
	})
	cfg := &trace.SpanConfig{}
	opt(cfg)
	if cfg.Kind != trace.SpanKindServer {
		t.Fatal("expected SpanKindServer")
	}
	if cfg.Attrs["key"] != "val" {
		t.Fatal("expected attr key=val")
	}
}
