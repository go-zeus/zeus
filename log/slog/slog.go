package slog

import (
	"context"
	"log/slog"

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
		// slog 无 Fatal 级别，降级为 Error 输出。
		// 进程退出由 log.Fatal 统一负责（确保 exit 前能 flush writer），
		// 此处不再调用 os.Exit，避免与 log.Fatal 形成双重退出。
		s.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	}
}

func (s *slogDriver) Close() error {
	return nil
}
