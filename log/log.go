package log

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
)

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// Field 日志字段
type Field struct {
	Key   string
	Value any
}

// Writer 日志写入器接口（实现者实现此接口）
type Writer interface {
	Log(ctx context.Context, level Level, msg string, fields ...Field)
	Close() error
}

// Logger 日志器（用户 API）
type Logger struct {
	writer Writer
	fields []Field
}

// NewLogger 从写入器创建 Logger
func NewLogger(w Writer) *Logger {
	return &Logger{writer: w}
}

// Debug 输出 DEBUG 级日志，msg 支持 fmt.Sprintf 格式化占位符。
func (l *Logger) Debug(msg string, args ...any) {
	l.Log(context.Background(), LevelDebug, fmt.Sprintf(msg, args...))
}

// Info 输出 INFO 级日志，msg 支持 fmt.Sprintf 格式化占位符。
func (l *Logger) Info(msg string, args ...any) {
	l.Log(context.Background(), LevelInfo, fmt.Sprintf(msg, args...))
}

// Warn 输出 WARN 级日志，msg 支持 fmt.Sprintf 格式化占位符。
func (l *Logger) Warn(msg string, args ...any) {
	l.Log(context.Background(), LevelWarn, fmt.Sprintf(msg, args...))
}

// Error 输出 ERROR 级日志，msg 支持 fmt.Sprintf 格式化占位符。
func (l *Logger) Error(msg string, args ...any) {
	l.Log(context.Background(), LevelError, fmt.Sprintf(msg, args...))
}

// Fatal 输出 FATAL 级日志后调用 os.Exit(1)（与 slog.Fatal 行为一致）。
func (l *Logger) Fatal(v ...any) {
	l.Log(context.Background(), LevelFatal, fmt.Sprint(v...))
	os.Exit(1)
}

// Log 通用入口：用户自定义 Level + ctx + msg + fields。
// ctx 用于自动从 baggage 提取 cluster / tenant 等 K-V 写成 Field（参见 propagation 包）。
func (l *Logger) Log(ctx context.Context, level Level, msg string, fields ...Field) {
	allFields := append(l.fields, fields...)
	// 自动从 context 提取集群标记和 baggage entries，注入为 Field
	//   - cluster：非默认集群才记录，避免污染 default 流量日志
	//   - baggage：用户自定义 K-V 全部写成 Field（跳过 zeus.cluster，避免与 cluster 重复）
	if ctx != nil {
		autoFields := autoFieldsFromContext(ctx)
		if len(autoFields) > 0 {
			allFields = append(autoFields, allFields...)
		}
	}
	l.writer.Log(ctx, level, msg, allFields...)
}

// autoFieldsFromContext 从 ctx 自动提取 cluster + baggage 作为 Field
//
// 顺序约定：cluster 排在最前（若非默认），baggage entries 按插入顺序追加。
// zeus.cluster 在 baggage 中存在时跳过（已被 cluster 字段表达，避免重复）。
func autoFieldsFromContext(ctx context.Context) []Field {
	var out []Field
	if c := routing.FromContext(ctx); !routing.IsDefault(c) {
		out = append(out, Field{Key: "cluster", Value: c})
	}
	bag := propagation.FromContext(ctx)
	if bag == nil {
		return out
	}
	for _, e := range bag.Entries() {
		if e.Key == routing.BagKey {
			continue // 已由 cluster 字段表达
		}
		out = append(out, Field{Key: e.Key, Value: e.Value})
	}
	return out
}

// With 返回带预设 fields 的派生 Logger（每次调用产生新实例，原 Logger 不变）。
// 用于把 request_id / user_id 等通用字段固定到 logger，避免每条日志重复传参。
func (l *Logger) With(fields ...Field) *Logger {
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)
	return &Logger{writer: l.writer, fields: newFields}
}

// Close 释放底层 Writer 资源（如 file_rotate 的句柄）。重复调用安全。
func (l *Logger) Close() error {
	return l.writer.Close()
}

// 包级便捷函数（使用默认 Logger）
// 使用 atomic.Pointer 存储 defaultLogger，避免 SetDefault 与 Default 并发访问的数据竞争
var defaultLogger atomic.Pointer[Logger]

func init() {
	defaultLogger.Store(NewLogger(newStdWriter()))
}

func SetDefault(l *Logger) {
	if l == nil {
		return // 拒绝 nil，避免后续 Default 调用 panic
	}
	defaultLogger.Store(l)
}

func Default() *Logger {
	return defaultLogger.Load()
}

// Debug 包级快捷函数：以默认 Logger 输出 DEBUG 级日志。
func Debug(msg string, args ...any) { defaultLogger.Load().Debug(msg, args...) }

// Info 包级快捷函数：以默认 Logger 输出 INFO 级日志。
func Info(msg string, args ...any) { defaultLogger.Load().Info(msg, args...) }

// Warn 包级快捷函数：以默认 Logger 输出 WARN 级日志。
func Warn(msg string, args ...any) { defaultLogger.Load().Warn(msg, args...) }

// Error 包级快捷函数：以默认 Logger 输出 ERROR 级日志。
func Error(msg string, args ...any) { defaultLogger.Load().Error(msg, args...) }

// Fatal 包级快捷函数：输出日志后调用 os.Exit(1)（与 slog.Fatal 一致）。
func Fatal(v ...any) { defaultLogger.Load().Fatal(v...) }

// stdWriter 内置标准输出写入器（零依赖兜底）
type stdWriter struct{}

// 编译期检查 stdWriter 实现了 Writer 接口
var _ Writer = (*stdWriter)(nil)

func newStdWriter() Writer { return &stdWriter{} }

func (s *stdWriter) Log(_ context.Context, level Level, msg string, fields ...Field) {
	var levelStr string
	switch level {
	case LevelDebug:
		levelStr = "DEBUG"
	case LevelInfo:
		levelStr = "INFO"
	case LevelWarn:
		levelStr = "WARN"
	case LevelError:
		levelStr = "ERROR"
	case LevelFatal:
		levelStr = "FATAL"
	}
	if len(fields) > 0 {
		fmt.Printf("[%s] %s %v\n", levelStr, msg, fields)
	} else {
		fmt.Printf("[%s] %s\n", levelStr, msg)
	}
}

func (s *stdWriter) Close() error { return nil }
