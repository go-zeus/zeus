package noop_test

import (
	"context"
	"testing"

	"github.com/go-zeus/zeus/trace"
	"github.com/go-zeus/zeus/trace/noop"
)

func TestNew(t *testing.T) {
	tr := noop.New()
	if tr == nil {
		t.Fatal("expected non-nil Tracer")
	}
}

func TestStartSpan(t *testing.T) {
	tr := noop.New()
	ctx, span := tr.StartSpan(context.Background(), "test")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestSpanOperations(t *testing.T) {
	tr := noop.New()
	_, span := tr.StartSpan(context.Background(), "test")
	span.SetAttributes(map[string]string{"key": "val"})
	span.SetName("renamed")
	span.RecordError(nil)
	if span.IsRecording() {
		t.Fatal("noop span should not be recording")
	}
	span.End()
}

func TestClose(t *testing.T) {
	tr := noop.New()
	if err := tr.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTracerInterface(t *testing.T) {
	// 验证 noop.New() 返回值满足 trace.Tracer 接口
	var _ trace.Tracer = noop.New()
}
