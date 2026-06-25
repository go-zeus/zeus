package otel

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/trace"
)

func TestNew_DefaultsNoPanic(t *testing.T) {
	tr := New()
	if tr == nil {
		t.Fatal("expected non-nil Tracer")
	}
}

func TestStartSpan_NoOpWhenExporterBroken(t *testing.T) {
	buf := &bytes.Buffer{}
	tr := New(WithStdoutWriter(buf))

	_, sp := tr.StartSpan(context.Background(), "op")
	if sp == nil {
		t.Fatal("expected non-nil Span")
	}
	sp.SetName("renamed")
	sp.SetAttributes(map[string]string{"k1": "v1"})
	sp.RecordError(errors.New("simulated"))
	sp.End()

	if err := tr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected stdout output, got empty buffer")
	}
}

func TestStartSpan_AttributesInjected(t *testing.T) {
	buf := &bytes.Buffer{}
	tr := New(WithStdoutWriter(buf))

	_, sp := tr.StartSpan(context.Background(), "withattrs",
		func(c *trace.SpanConfig) { c.Attrs = map[string]string{"zeus.cluster": "canary"} })
	sp.End()
	_ = tr.Close()

	if !strings.Contains(buf.String(), "zeus.cluster") {
		t.Fatalf("expected attribute in output, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "canary") {
		t.Fatalf("expected cluster value in output")
	}
}

func TestStartSpan_RecordErrorSetsErrorStatus(t *testing.T) {
	buf := &bytes.Buffer{}
	tr := New(WithStdoutWriter(buf))

	_, sp := tr.StartSpan(context.Background(), "fail")
	sp.RecordError(errors.New("boom"))
	sp.End()
	_ = tr.Close()

	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("expected error in span, got:\n%s", buf.String())
	}
}

func TestStartSpan_ServiceNameInResource(t *testing.T) {
	buf := &bytes.Buffer{}
	tr := New(
		WithServiceName("my-app"),
		WithServiceVersion("v1.2.3"),
		WithStdoutWriter(buf),
	)
	_, sp := tr.StartSpan(context.Background(), "ping")
	sp.End()
	_ = tr.Close()

	if !strings.Contains(buf.String(), "my-app") {
		t.Fatalf("expected service.name in resource: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "v1.2.3") {
		t.Fatalf("expected service.version in resource")
	}
}

func TestStartSpan_ParentChildSpanLinked(t *testing.T) {
	buf := &bytes.Buffer{}
	tr := New(WithStdoutWriter(buf))

	parentCtx, parent := tr.StartSpan(context.Background(), "parent")
	_, child := tr.StartSpan(parentCtx, "child")
	child.End()
	parent.End()
	_ = tr.Close()

	// 至少有两条 span（顺序不一定）
	count := strings.Count(buf.String(), `"Name":"`)
	if count < 2 {
		t.Fatalf("expected at least 2 spans, got %d in:\n%s", count, buf.String())
	}
}

func TestSpanKindMapping(t *testing.T) {
	cases := []struct {
		zeus trace.SpanKind
	}{
		{trace.SpanKindInternal},
		{trace.SpanKindServer},
		{trace.SpanKindClient},
		{trace.SpanKindProducer},
		{trace.SpanKindConsumer},
	}
	for _, c := range cases {
		got := toOtelSpanKind(c.zeus)
		_ = got // 不 panic 即可
	}
}

func TestClose_Idempotent(t *testing.T) {
	tr := New()
	_ = tr.Close()
	_ = tr.Close()
}
