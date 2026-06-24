package noop

import (
	"context"

	"github.com/go-zeus/zeus/trace"
)

type noopTracer struct{}
type noopSpan struct{}

// New 创建 noop Tracer
func New() trace.Tracer { return &noopTracer{} }

func (n *noopTracer) StartSpan(ctx context.Context, _ string, _ ...trace.SpanOption) (context.Context, trace.Span) {
	return ctx, &noopSpan{}
}

func (n *noopTracer) Close() error { return nil }

func (n *noopSpan) End()                              {}
func (n *noopSpan) SetAttributes(_ map[string]string) {}
func (n *noopSpan) SetName(_ string)                  {}
func (n *noopSpan) RecordError(_ error)               {}
func (n *noopSpan) IsRecording() bool                 { return false }
