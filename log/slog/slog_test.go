package slog

import (
	"context"
	"testing"

	"github.com/go-zeus/zeus/log"
)

// TestNewSlog 测试创建 slog 写入器
func TestNewSlog(t *testing.T) {
	d := NewSlog()
	if d == nil {
		t.Fatal("NewSlog returned nil")
	}
}

// TestLog_Levels 测试各日志级别不会 panic
func TestLog_Levels(t *testing.T) {
	d := NewSlog()
	ctx := context.Background()

	levels := []struct {
		name  string
		level log.Level
	}{
		{"Debug", log.LevelDebug},
		{"Info", log.LevelInfo},
		{"Warn", log.LevelWarn},
		{"Error", log.LevelError},
	}

	for _, tt := range levels {
		t.Run(tt.name, func(t *testing.T) {
			// 仅验证不会 panic
			d.Log(ctx, tt.level, "test "+tt.name+" message")
		})
	}
}

// TestLog_WithFields 测试带字段的日志输出
func TestLog_WithFields(t *testing.T) {
	d := NewSlog()
	ctx := context.Background()

	fields := []log.Field{
		{Key: "service", Value: "zeus"},
		{Key: "port", Value: 8080},
		{Key: "debug", Value: true},
	}

	// 仅验证不会 panic
	d.Log(ctx, log.LevelInfo, "test with fields", fields...)
}

// TestLog_EmptyFields 测试空字段日志
func TestLog_EmptyFields(t *testing.T) {
	d := NewSlog()
	ctx := context.Background()

	// 不传 fields 参数
	d.Log(ctx, log.LevelInfo, "no fields")

	// 传空的 fields
	d.Log(ctx, log.LevelInfo, "empty fields", []log.Field{}...)
}

// TestClose 测试 Close 返回 nil
func TestClose(t *testing.T) {
	d := NewSlog()
	if err := d.Close(); err != nil {
		t.Fatalf("Close should return nil, got %v", err)
	}
}

// 确认 slogDriver 实现了 log.Writer 接口
var _ log.Writer = (*slogDriver)(nil)
