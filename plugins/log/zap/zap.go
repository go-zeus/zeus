// Package zap 提供基于 go.uber.org/zap 的 log.Writer 实现。
//
// 设计要点：
//   - 默认用 NewProduction()（JSON 输出 + Info 级别 + sampling）
//   - cluster 字段已被 log.Logger.Log 自动注入到 fields，这里直接转 zap.Field
//   - LevelFatal 映射到 zap.FatalLevel + os.Exit(1)，与 slog 行为对齐
//
// 用法：
//
//	w, err := zap.New()
//	if err != nil { panic(err) }
//	logger := log.NewLogger(w)
//	defer logger.Close()
//
// 自定义 zap 实例：
//
//	zapLogger := zap.NewExample()
//	w := zap.NewWith(zapLogger)
package zap

import (
	"context"
	"os"
	"sync/atomic"

	"github.com/go-zeus/zeus/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Option 函数式选项
type Option func(*writer)

// WithLevel 设置最小日志级别（默认 zap.InfoLevel）。
// 低于此级别的日志会被丢弃。
func WithLevel(level zapcore.Level) Option {
	return func(w *writer) {
		w.minLevel.Store(int32(level))
	}
}

// 编译期检查 writer 实现 log.Writer
var _ log.Writer = (*writer)(nil)

type writer struct {
	logger   *zap.Logger
	minLevel atomic.Int32 // zapcore.Level (int8)，用 atomic 避免 Set/Log 竞争
}

// New 创建 zap Writer，使用 NewProduction 配置（JSON、Info 级别、采样）。
// 失败场景几乎只会在配置 JSON 非法时出现。
func New(opts ...Option) (log.Writer, error) {
	z, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}
	w := &writer{logger: z}
	w.minLevel.Store(int32(zap.InfoLevel))
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

// NewWith 用外部 *zap.Logger 创建 Writer。
// 不接管其生命周期；Close 仍会调用 Sync 保证缓冲落盘，但不 Close 底层 logger。
// 用于：复用全局 logger、注入测试 logger（NewExample/NewNop）。
func NewWith(z *zap.Logger) log.Writer {
	if z == nil {
		z = zap.NewNop()
	}
	w := &writer{logger: z}
	w.minLevel.Store(int32(zap.InfoLevel))
	return w
}

// Log 把 zeus 日志事件转发到 zap。
//
// 注意：cluster 字段已由 log.Logger.Log 自动注入到 fields，无需在此重复提取。
func (w *writer) Log(_ context.Context, level log.Level, msg string, fields ...log.Field) {
	lvl := toZapLevel(level)
	if lvl < zapcore.Level(w.minLevel.Load()) {
		return
	}
	zfields := make([]zap.Field, 0, len(fields))
	for _, f := range fields {
		zfields = append(zfields, zap.Any(f.Key, f.Value))
	}

	ce := w.logger.Check(lvl, msg)
	if ce == nil {
		// 高级别日志被采样策略丢弃
		return
	}
	ce.Write(zfields...)

	if level == log.LevelFatal {
		// Fatal: 先 Sync 保证 buffer 落盘，再 os.Exit(1)
		_ = w.logger.Sync()
		os.Exit(1)
	}
}

// Close flush 缓冲。
func (w *writer) Close() error {
	return w.logger.Sync()
}

// toZapLevel zeus Level → zapcore.Level
func toZapLevel(level log.Level) zapcore.Level {
	switch level {
	case log.LevelDebug:
		return zap.DebugLevel
	case log.LevelInfo:
		return zap.InfoLevel
	case log.LevelWarn:
		return zap.WarnLevel
	case log.LevelError:
		return zap.ErrorLevel
	case log.LevelFatal:
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}
