package zap

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/routing"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// captureEncoder 用 in-memory encoder + atomic.Level 包一个 zap.Logger，
// 便于测试断言输出内容。
func captureLogger(t *testing.T, level zapcore.Level) (*zap.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(buf),
		level,
	)
	return zap.New(core), buf
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid json line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestNew_ProductionDefaults(t *testing.T) {
	w, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil Writer")
	}
	_ = w.Close()
}

func TestNewWith_NilSafe(t *testing.T) {
	// nil → 兜底为 Nop，不 panic
	w := NewWith(nil)
	w.Log(context.Background(), log.LevelInfo, "noop ok")
	_ = w.Close()
}

func TestLog_LevelFiltering(t *testing.T) {
	z, buf := captureLogger(t, zapcore.InfoLevel)
	w := NewWith(z)

	// Debug 应被过滤（minLevel=Info）
	w.Log(context.Background(), log.LevelDebug, "debug-msg")
	w.Log(context.Background(), log.LevelInfo, "info-msg")
	_ = w.Close()

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (Debug filtered), got %d: %s", len(lines), buf.String())
	}
	if lines[0]["msg"] != "info-msg" {
		t.Fatalf("expected info-msg, got %v", lines[0]["msg"])
	}
}

func TestLog_LevelMapping(t *testing.T) {
	z, buf := captureLogger(t, zapcore.DebugLevel)
	w := NewWith(z)
	// 默认 writer.minLevel=Info，需放开到 Debug 才能让 Debug 通过
	WithLevel(zapcore.DebugLevel)(w.(*writer))

	w.Log(context.Background(), log.LevelDebug, "d")
	w.Log(context.Background(), log.LevelInfo, "i")
	w.Log(context.Background(), log.LevelWarn, "w")
	w.Log(context.Background(), log.LevelError, "e")
	_ = w.Close()

	lines := decodeLines(t, buf)
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d; raw=%s", len(lines), buf.String())
	}
	want := []string{"debug", "info", "warn", "error"}
	for i, want := range want {
		if lines[i]["level"] != want {
			t.Fatalf("line %d: level=%v want %s", i, lines[i]["level"], want)
		}
	}
}

func TestLog_FieldsInjected(t *testing.T) {
	z, buf := captureLogger(t, zapcore.InfoLevel)
	w := NewWith(z)

	w.Log(context.Background(), log.LevelInfo, "with-attrs",
		log.Field{Key: "user", Value: "alice"},
		log.Field{Key: "age", Value: 30},
	)
	_ = w.Close()

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0]["user"] != "alice" {
		t.Fatalf("expected user=alice, got %v", lines[0]["user"])
	}
	if lines[0]["age"].(float64) != 30 {
		t.Fatalf("expected age=30, got %v", lines[0]["age"])
	}
}

func TestLog_ClusterAutoInjected(t *testing.T) {
	// 模拟 log.Logger.Log 的 cluster 自动注入行为：
	// 调用方 routing.WithContext 后 log.Logger.Log 会 prepend cluster Field
	z, buf := captureLogger(t, zapcore.InfoLevel)
	w := NewWith(z)

	// 直接调用 writer（绕过 Logger.Log），手动 prepend cluster field 模拟
	ctx := routing.WithCluster(context.Background(), "canary")
	logger := log.NewLogger(w)
	logger.Log(ctx, log.LevelInfo, "cluster-test")
	_ = w.Close()

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0]["cluster"] != "canary" {
		t.Fatalf("expected cluster=canary, got %v", lines[0]["cluster"])
	}
}

func TestLog_WithLevel_Option(t *testing.T) {
	z, buf := captureLogger(t, zapcore.DebugLevel)
	w := NewWith(z)
	// 通过 Option 重设 minLevel 到 Error，Info 应被过滤
	WithLevel(zapcore.ErrorLevel)(w.(*writer))

	w.Log(context.Background(), log.LevelInfo, "filtered")
	w.Log(context.Background(), log.LevelError, "kept")
	_ = w.Close()

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (Info filtered), got %d: %s", len(lines), buf.String())
	}
	if lines[0]["msg"] != "kept" {
		t.Fatalf("expected 'kept', got %v", lines[0]["msg"])
	}
}

func TestToZapLevel_AllMapped(t *testing.T) {
	cases := []struct {
		in   log.Level
		want zapcore.Level
	}{
		{log.LevelDebug, zap.DebugLevel},
		{log.LevelInfo, zap.InfoLevel},
		{log.LevelWarn, zap.WarnLevel},
		{log.LevelError, zap.ErrorLevel},
		{log.LevelFatal, zap.FatalLevel},
	}
	for _, c := range cases {
		if got := toZapLevel(c.in); got != c.want {
			t.Errorf("toZapLevel(%v)=%v want %v", c.in, got, c.want)
		}
	}
}
