package noop

import (
	"context"

	"github.com/go-zeus/zeus/trace"
)

type noopTracer struct{}
type noopSpan struct{}

// New 创建 noop Tracer
func New() trace.Tracer { return &noopTracer{} }

// IsNoop 判断 tracer 是否为 noop 实现。
//
// 供埋点热路径（如 cache/memory）在 noop 时跳过 attrs/span 构造开销：
// 调用方在构造期探测一次，存 bool 标志，避免每次操作分配 span + labels map。
func IsNoop(t trace.Tracer) bool {
	_, ok := t.(*noopTracer)
	return ok
}

// Span 是复用的 noop Span 实例，供调用方跳过 span 构造时直接返回（零分配）。
//
// 用法：if !traceEnabled { return ctx, noop.Span }
var Span trace.Span = &noopSpan{}

func (n *noopTracer) StartSpan(ctx context.Context, _ string, _ ...trace.SpanOption) (context.Context, trace.Span) {
	return ctx, Span
}

func (n *noopTracer) Close() error { return nil }

func (n *noopSpan) End()                              {}
func (n *noopSpan) SetAttributes(_ map[string]string) {}
func (n *noopSpan) SetName(_ string)                  {}
func (n *noopSpan) RecordError(_ error)               {}
func (n *noopSpan) IsRecording() bool                 { return false }
