package file_rotate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-zeus/zeus/log"
)

// newTestWriter 在临时目录创建 writer，自动清理
func newTestWriter(t *testing.T, opts ...Option) (*writer, string) {
	t.Helper()
	dir := t.TempDir()
	filename := filepath.Join(dir, "app.log")
	w, err := New(filename, opts...)
	if err != nil {
		t.Fatalf("New err: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w.(*writer), filename
}

// TestNew_Basic 基础构造
func TestNew_Basic(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "sub", "app.log")
	w, err := New(filename)
	if err != nil {
		t.Fatalf("New err: %v", err)
	}
	defer w.Close()

	// 父目录自动创建
	if _, err := os.Stat(filepath.Join(dir, "sub")); err != nil {
		t.Errorf("parent dir not created: %v", err)
	}
}

// TestNew_EmptyFilename 空文件名报错
func TestNew_EmptyFilename(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Error("New(\"\") should error")
	}
}

// TestLog_JSONFormat JSON 格式正确
func TestLog_JSONFormat(t *testing.T) {
	w, filename := newTestWriter(t, WithFormat(FormatJSON))

	ctx := context.Background()
	w.Log(ctx, log.LevelInfo, "user logged in",
		log.Field{Key: "user_id", Value: 123},
		log.Field{Key: "ip", Value: "10.0.0.1"},
	)

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data[:len(data)-1], &m); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, data)
	}
	if m["msg"] != "user logged in" {
		t.Errorf("msg = %v", m["msg"])
	}
	if m["level"] != "INFO" {
		t.Errorf("level = %v", m["level"])
	}
	if m["user_id"].(float64) != 123 {
		t.Errorf("user_id = %v", m["user_id"])
	}
	if m["ip"] != "10.0.0.1" {
		t.Errorf("ip = %v", m["ip"])
	}
	if _, ok := m["time"]; !ok {
		t.Error("time field missing")
	}
}

// TestLog_TextFormat TEXT 格式正确
func TestLog_TextFormat(t *testing.T) {
	w, filename := newTestWriter(t, WithFormat(FormatText))

	ctx := context.Background()
	w.Log(ctx, log.LevelInfo, "hello", log.Field{Key: "k", Value: "v"})

	data, _ := os.ReadFile(filename)
	line := strings.TrimRight(string(data), "\n")
	parts := strings.Split(line, "\t")
	// 期望 [time, INFO, hello, k=v]
	if len(parts) < 4 {
		t.Fatalf("text format parts = %v, want at least 4", parts)
	}
	if parts[1] != "INFO" {
		t.Errorf("level = %q, want INFO", parts[1])
	}
	if parts[2] != "hello" {
		t.Errorf("msg = %q, want hello", parts[2])
	}
	if parts[3] != "k=v" {
		t.Errorf("field = %q, want k=v", parts[3])
	}
}

// TestLog_LevelFilter 低于 minLevel 的日志被丢弃
func TestLog_LevelFilter(t *testing.T) {
	w, filename := newTestWriter(t, WithMinLevel(log.LevelWarn))

	ctx := context.Background()
	w.Log(ctx, log.LevelDebug, "debug msg") // 应被丢弃
	w.Log(ctx, log.LevelInfo, "info msg")   // 应被丢弃
	w.Log(ctx, log.LevelWarn, "warn msg")   // 输出
	w.Log(ctx, log.LevelError, "error msg") // 输出

	data, _ := os.ReadFile(filename)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (warn + error)", len(lines))
	}
	if !strings.Contains(lines[0], "warn msg") {
		t.Errorf("line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], "error msg") {
		t.Errorf("line 1 = %q", lines[1])
	}
}

// TestLog_Concurrent 并发写入无 race + 数据完整
func TestLog_Concurrent(t *testing.T) {
	w, filename := newTestWriter(t)

	ctx := context.Background()
	const N = 100
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			w.Log(ctx, log.LevelInfo, "concurrent", log.Field{Key: "i", Value: i})
		}(i)
	}
	for i := 0; i < N; i++ {
		<-done
	}

	data, _ := os.ReadFile(filename)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != N {
		t.Errorf("lines = %d, want %d", len(lines), N)
	}
	// 每行都是合法 JSON
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("line %d invalid JSON: %v", i, err)
		}
	}
}

// TestOptions 应用 lumberjack 选项
func TestOptions(t *testing.T) {
	w, _ := newTestWriter(t,
		WithMaxSize(10),
		WithMaxBackups(3),
		WithMaxAge(7),
		WithCompress(true),
		WithLocalTime(true),
	)
	if w.lumberjack.MaxSize != 10 {
		t.Errorf("MaxSize = %d, want 10", w.lumberjack.MaxSize)
	}
	if w.lumberjack.MaxBackups != 3 {
		t.Errorf("MaxBackups = %d, want 3", w.lumberjack.MaxBackups)
	}
	if w.lumberjack.MaxAge != 7 {
		t.Errorf("MaxAge = %d, want 7", w.lumberjack.MaxAge)
	}
	if !w.lumberjack.Compress {
		t.Error("Compress = false, want true")
	}
	if !w.lumberjack.LocalTime {
		t.Error("LocalTime = false, want true")
	}
}

// TestLevelString 级别字符串
func TestLevelString(t *testing.T) {
	cases := []struct {
		level log.Level
		want  string
	}{
		{log.LevelDebug, "DEBUG"},
		{log.LevelInfo, "INFO"},
		{log.LevelWarn, "WARN"},
		{log.LevelError, "ERROR"},
		{log.LevelFatal, "FATAL"},
		{log.Level(99), "INFO"}, // 未知级别兜底
	}
	for _, tc := range cases {
		if got := levelString(tc.level); got != tc.want {
			t.Errorf("levelString(%v) = %q, want %q", tc.level, got, tc.want)
		}
	}
}

// TestDefaultConstants 默认常量合理
func TestDefaultConstants(t *testing.T) {
	if DefaultMaxSize != 100 {
		t.Errorf("DefaultMaxSize = %v, want 100", DefaultMaxSize)
	}
	if DefaultMaxBackups != 7 {
		t.Errorf("DefaultMaxBackups = %v, want 7", DefaultMaxBackups)
	}
	if DefaultMaxAge != 30 {
		t.Errorf("DefaultMaxAge = %v, want 30", DefaultMaxAge)
	}
}

// TestClose_Idempotent Close 多次调用安全
func TestClose_Idempotent(t *testing.T) {
	w, _ := newTestWriter(t)
	if err := w.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close should be no-op, got: %v", err)
	}
}

// TestFormatJSON_TimeFormat JSON 中 time 字段是 RFC3339Nano
func TestFormatJSON_TimeFormat(t *testing.T) {
	w, _ := newTestWriter(t)
	line := w.formatJSON(log.LevelInfo, "test", nil)
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if ts, ok := m["time"].(string); ok {
		if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
			t.Errorf("time %q not RFC3339Nano: %v", ts, err)
		}
	} else {
		t.Errorf("time field not string: %T", m["time"])
	}
}
