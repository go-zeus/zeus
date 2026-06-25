package slog

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-zeus/zeus/log"
)

// 编译期检查 slogDriver 实现了 log.Writer 接口
var _ log.Writer = (*slogDriver)(nil)

type slogDriver struct {
	logger *slog.Logger
}

// NewSlog 创建 slog 日志写入器
func NewSlog() log.Writer {
	return &slogDriver{logger: slog.Default()}
}

func (s *slogDriver) Log(_ context.Context, level log.Level, msg string, fields ...log.Field) {
	attrs := make([]slog.Attr, 0, len(fields))
	for _, f := range fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	switch level {
	case log.LevelDebug:
		s.logger.LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
	case log.LevelInfo:
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
	case log.LevelWarn:
		s.logger.LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
	case log.LevelError:
		s.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	case log.LevelFatal:
		s.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
		os.Exit(1)
	}
}

func (s *slogDriver) Close() error {
	return nil
}
