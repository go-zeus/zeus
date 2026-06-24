package trace

import "context"

// SpanKind Span 类型
type SpanKind int

const (
	SpanKindInternal SpanKind = iota
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// SpanConfig Span 配置
type SpanConfig struct {
	Kind  SpanKind
	Attrs map[string]string
}

// SpanOption Span 选项
type SpanOption func(*SpanConfig)

// Span 链路追踪 Span 接口
type Span interface {
	// End 结束 span
	End()
	// SetAttributes 设置属性
	SetAttributes(attrs map[string]string)
	// SetName 设置名称
	SetName(name string)
	// RecordError 记录错误
	RecordError(err error)
	// IsRecording 是否正在记录
	IsRecording() bool
}

// Tracer 链路追踪器接口
type Tracer interface {
	// StartSpan 创建一个 span
	StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)
	// Close 关闭
	Close() error
}
