// Package otel 提供基于 go.opentelemetry.io/otel 的 Tracer 实现。
//
// 设计要点：
//   - 桥接 zeus/trace.Tracer 到 otel trace.Tracer（保留 ctx 注入语义）
//   - 资源属性：service.name/version + 自定义 attrs（按 OTel semconv 约定）
//   - 默认采样：AlwaysSample（生产可改 ParentBased(TraceIDRatioBased(0.1))）
//   - 默认 exporter：stdout（debug 用，生产应替换为 OTLP）
//
// 用法：
//
//	tracer := otel.New(
//	    otel.WithServiceName("my-app"),
//	    otel.WithExporter(otlpExporter),
//	)
//	chain := middleware.NewChain(tracing.New(tracer))
package otel

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/go-zeus/zeus/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Option 函数式选项
type Option func(*tracer)

// WithServiceName 设置 service.name 资源属性（OTel semconv）。
// 默认 "zeus"。
func WithServiceName(name string) Option {
	return func(t *tracer) {
		if name != "" {
			t.serviceName = name
		}
	}
}

// WithServiceVersion 设置 service.version 资源属性。
func WithServiceVersion(v string) Option {
	return func(t *tracer) { t.serviceVersion = v }
}

// WithResourceAttrs 追加资源属性（OTel semconv 风格：host.name, deployment.environment 等）。
func WithResourceAttrs(attrs map[string]string) Option {
	return func(t *tracer) {
		for k, v := range attrs {
			t.resourceAttrs[k] = v
		}
	}
}

// WithSampler 自定义采样器。
// 不设置时默认 sdktrace.AlwaysSample()。
func WithSampler(s sdktrace.Sampler) Option {
	return func(t *tracer) {
		if s != nil {
			t.sampler = s
		}
	}
}

// WithExporter 自定义 SpanExporter（如 OTLP gRPC/HTTP、Jaeger、stdout）。
// 不设置时默认 stdout exporter，写到 stderr（仅 debug 用）。
func WithExporter(e sdktrace.SpanExporter) Option {
	return func(t *tracer) {
		if e != nil {
			t.exporter = e
		}
	}
}

// WithStdoutWriter 仅在未指定 WithExporter 时生效。
// 控制 stdout exporter 的输出目标（默认 os.Stderr）。
func WithStdoutWriter(w io.Writer) Option {
	return func(t *tracer) {
		if w != nil {
			t.stdoutWriter = w
		}
	}
}

// 编译期检查 tracer 实现 trace.Tracer
var _ trace.Tracer = (*tracer)(nil)

type tracer struct {
	serviceName    string
	serviceVersion string
	resourceAttrs  map[string]string
	sampler        sdktrace.Sampler
	exporter       sdktrace.SpanExporter
	stdoutWriter   io.Writer

	once     sync.Once
	provider *sdktrace.TracerProvider
	inner    oteltrace.Tracer
	initErr  error
}

// New 创建 OTel Tracer。
// 拨号是惰性的：首次 StartSpan 时才初始化 TracerProvider（含 exporter 连接）。
func New(opts ...Option) trace.Tracer {
	t := &tracer{
		serviceName:   "zeus",
		resourceAttrs: make(map[string]string),
		sampler:       sdktrace.AlwaysSample(),
		stdoutWriter:  os.Stderr,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// init 惰性初始化 provider。多次调用幂等。
func (t *tracer) init() error {
	t.once.Do(func() {
		// 资源属性（OTel semconv）
		kvs := []attribute.KeyValue{
			semconv.ServiceName(t.serviceName),
		}
		if t.serviceVersion != "" {
			kvs = append(kvs, semconv.ServiceVersion(t.serviceVersion))
		}
		for k, v := range t.resourceAttrs {
			kvs = append(kvs, attribute.String(k, v))
		}
		res, err := sdkresource.New(context.Background(), sdkresource.WithAttributes(kvs...))
		if err != nil {
			t.initErr = fmt.Errorf("otel: build resource: %w", err)
			return
		}

		// exporter：用户未指定则用 stdout
		exp := t.exporter
		if exp == nil {
			exp, err = stdouttrace.New(stdouttrace.WithWriter(t.stdoutWriter))
			if err != nil {
				t.initErr = fmt.Errorf("otel: stdout exporter: %w", err)
				return
			}
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(t.sampler),
			sdktrace.WithResource(res),
			sdktrace.WithBatcher(exp),
		)
		// 设置全局 provider，便于第三方 OTel-instrumented 库自动接入
		otel.SetTracerProvider(tp)
		t.provider = tp
		t.inner = tp.Tracer(t.serviceName)
	})
	return t.initErr
}

// StartSpan 创建 span 并把 otel span 注入 ctx，便于下游 otel-aware 库继承父链。
func (t *tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanOption) (context.Context, trace.Span) {
	if err := t.init(); err != nil {
		// 初始化失败：返回 noop span，业务流程不受影响
		return ctx, noopSpan{}
	}
	cfg := trace.SpanConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	otelOpts := []oteltrace.SpanStartOption{oteltrace.WithSpanKind(toOtelSpanKind(cfg.Kind))}
	if len(cfg.Attrs) > 0 {
		otelOpts = append(otelOpts, oteltrace.WithAttributes(toOtelAttrs(cfg.Attrs)...))
	}
	newCtx, s := t.inner.Start(ctx, name, otelOpts...)
	return newCtx, &span{inner: s}
}

func (t *tracer) Close() error {
	if t.provider == nil {
		return nil
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.provider.Shutdown(shutCtx)
}

// span 桥接 zeus trace.Span 到 otel trace.Span
type span struct {
	inner oteltrace.Span
}

func (s *span) End() { s.inner.End() }

func (s *span) SetAttributes(attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}
	s.inner.SetAttributes(toOtelAttrs(attrs)...)
}

func (s *span) SetName(name string) { s.inner.SetName(name) }

func (s *span) RecordError(err error) {
	if err == nil {
		return
	}
	s.inner.RecordError(err)
	s.inner.SetStatus(codes.Error, err.Error())
}

func (s *span) IsRecording() bool { return s.inner.IsRecording() }

// noopSpan 在 provider 初始化失败时使用
type noopSpan struct{}

func (noopSpan) End()                            {}
func (noopSpan) SetAttributes(map[string]string) {}
func (noopSpan) SetName(string)                  {}
func (noopSpan) RecordError(error)               {}
func (noopSpan) IsRecording() bool               { return false }

// toOtelSpanKind zeus SpanKind → otel SpanKind
func toOtelSpanKind(k trace.SpanKind) oteltrace.SpanKind {
	switch k {
	case trace.SpanKindServer:
		return oteltrace.SpanKindServer
	case trace.SpanKindClient:
		return oteltrace.SpanKindClient
	case trace.SpanKindProducer:
		return oteltrace.SpanKindProducer
	case trace.SpanKindConsumer:
		return oteltrace.SpanKindConsumer
	default:
		return oteltrace.SpanKindInternal
	}
}

// toOtelAttrs map[string]string → []attribute.KeyValue
func toOtelAttrs(attrs map[string]string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		out = append(out, attribute.String(k, v))
	}
	return out
}
