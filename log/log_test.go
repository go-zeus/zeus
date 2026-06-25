package log

import (
	"context"
	"sync"
	"testing"

	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
)

// ---- mock writer ----

type mockWriter struct {
	mu      sync.Mutex
	records []record
}

type record struct {
	level  Level
	msg    string
	fields []Field
}

func (m *mockWriter) Log(_ context.Context, level Level, msg string, fields ...Field) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record{level: level, msg: msg, fields: fields})
}

func (m *mockWriter) Close() error { return nil }

func (m *mockWriter) lastRecord() (record, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		return record{}, false
	}
	return m.records[len(m.records)-1], true
}

// ---- tests ----

func TestNewLogger(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)
	if l == nil {
		t.Fatal("NewLogger returned nil")
	}

	l.Info("hello")
	r, ok := d.lastRecord()
	if !ok {
		t.Fatal("no log record found")
	}
	if r.level != LevelInfo {
		t.Errorf("level = %v, want %v", r.level, LevelInfo)
	}
	if r.msg != "hello" {
		t.Errorf("msg = %q, want %q", r.msg, "hello")
	}
}

func TestDefaultFunctions(t *testing.T) {
	// 包级便捷函数不应 panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Debug panicked: %v", r)
			}
		}()
		Debug("test debug %s", "msg")
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Info panicked: %v", r)
			}
		}()
		Info("test info %s", "msg")
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Warn panicked: %v", r)
			}
		}()
		Warn("test warn %s", "msg")
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Error panicked: %v", r)
			}
		}()
		Error("test error %s", "msg")
	}()
}

func TestWithFields(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)

	child := l.With(Field{Key: "request_id", Value: "abc123"})
	if child == nil {
		t.Fatal("With returned nil")
	}

	child.Info("with fields")
	r, ok := d.lastRecord()
	if !ok {
		t.Fatal("no log record found")
	}

	// 验证字段被传递
	found := false
	for _, f := range r.fields {
		if f.Key == "request_id" && f.Value == "abc123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("fields = %v, want field request_id=abc123", r.fields)
	}
}

func TestStdWriter(t *testing.T) {
	d := newStdWriter()

	// 各级别不应 panic
	levels := []Level{
		LevelDebug,
		LevelInfo,
		LevelWarn,
		LevelError,
	}

	for _, lvl := range levels {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("stdWriter.Log level %v panicked: %v", lvl, r)
				}
			}()
			d.Log(context.Background(), lvl, "test message")
		}()
	}

	if err := d.Close(); err != nil {
		t.Errorf("stdWriter.Close() error: %v", err)
	}
}

// TestLogLevels 验证 Logger 的 Debug/Info/Warn/Error 各级别正常工作
func TestLogLevels(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)

	cases := []struct {
		logFunc func(string, ...any)
		level   Level
	}{
		{l.Debug, LevelDebug},
		{l.Info, LevelInfo},
		{l.Warn, LevelWarn},
		{l.Error, LevelError},
	}

	for _, tc := range cases {
		tc.logFunc("test message")

		r, ok := d.lastRecord()
		if !ok {
			t.Fatalf("%v 级别未产生日志记录", tc.level)
		}
		if r.level != tc.level {
			t.Errorf("level = %v, want %v", r.level, tc.level)
		}
		if r.msg != "test message" {
			t.Errorf("msg = %q, want %q", r.msg, "test message")
		}
	}
}

// findField 在 fields 中查找指定 key
func findField(fields []Field, key string) (Field, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// TestLog_AutoClusterFromContext 自动从 ctx 提取 cluster 写成 Field（已有行为）
func TestLog_AutoClusterFromContext(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)

	ctx := routing.WithCluster(context.Background(), "canary")
	l.Log(ctx, LevelInfo, "hello")

	r, ok := d.lastRecord()
	if !ok {
		t.Fatal("no record")
	}
	f, exists := findField(r.fields, "cluster")
	if !exists {
		t.Fatal("cluster field should be auto-injected")
	}
	if f.Value != "canary" {
		t.Errorf("cluster = %v, want canary", f.Value)
	}
}

// TestLog_AutoBaggageEntriesFromContext 自动从 ctx 提取 baggage entries 写成 Field
func TestLog_AutoBaggageEntriesFromContext(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)

	ctx := propagation.With(context.Background(), "tenant.id", "acme-corp")
	ctx = propagation.With(ctx, "feature.flag", "beta")
	l.Log(ctx, LevelInfo, "hello")

	r, ok := d.lastRecord()
	if !ok {
		t.Fatal("no record")
	}
	if f, found := findField(r.fields, "tenant.id"); !found || f.Value != "acme-corp" {
		t.Errorf("tenant.id field missing or wrong: %+v", f)
	}
	if f, found := findField(r.fields, "feature.flag"); !found || f.Value != "beta" {
		t.Errorf("feature.flag field missing or wrong: %+v", f)
	}
}

// TestLog_ClusterAndBaggage_NoDuplicate 当 ctx 同时有 cluster 和 baggage(zeus.cluster) 时，
// 不产生重复字段（zeus.cluster 在 baggage 中跳过）
func TestLog_ClusterAndBaggage_NoDuplicate(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)

	// routing.WithCluster 同时写入 ctx 和 Bag，所以两处都有 zeus.cluster
	ctx := routing.WithCluster(context.Background(), "canary")
	l.Log(ctx, LevelInfo, "hello")

	r, ok := d.lastRecord()
	if !ok {
		t.Fatal("no record")
	}
	// cluster Field 应该恰好 1 个
	count := 0
	for _, f := range r.fields {
		if f.Key == "cluster" || f.Key == routing.BagKey {
			count++
		}
	}
	if count != 1 {
		t.Errorf("cluster-related fields = %d, want 1 (no duplicate)", count)
	}
}

// TestLog_DefaultClusterNoField default cluster 不写成 Field（避免噪音）
func TestLog_DefaultClusterNoField(t *testing.T) {
	d := &mockWriter{}
	l := NewLogger(d)

	l.Log(context.Background(), LevelInfo, "hello")

	r, ok := d.lastRecord()
	if !ok {
		t.Fatal("no record")
	}
	if _, found := findField(r.fields, "cluster"); found {
		t.Error("default cluster should not produce a Field")
	}
}
