package database

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/trace"
)

// TestNewFromURL_Empty 空字符串返回 (nil, nil)
func TestNewFromURL_Empty(t *testing.T) {
	db, err := NewFromURL("", nil, nil)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if db != nil {
		t.Errorf("db = %v, want nil", db)
	}
}

// TestNewFromURL_NoScheme 无 scheme 返回明确错误
func TestNewFromURL_NoScheme(t *testing.T) {
	_, err := NewFromURL("no-scheme-here", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("err = %v", err)
	}
}

// TestNewFromURL_UnknownScheme 未知 scheme 返回错误
func TestNewFromURL_UnknownScheme(t *testing.T) {
	_, err := NewFromURL("nonexistent://localhost", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown scheme") {
		t.Errorf("err = %v", err)
	}
}

// TestNewFromURL_RegisteredResolver 注册的 resolver 能被调用，tracer/meter 透传
func TestNewFromURL_RegisteredResolver(t *testing.T) {
	var gotURL string
	var gotTracer trace.Tracer
	var gotMeter metrics.Meter

	RegisterResolver("test-db-scheme", func(rawURL string, t trace.Tracer, m metrics.Meter) (DB, error) {
		gotURL = rawURL
		gotTracer = t
		gotMeter = m
		return nil, nil
	})
	defer delete(resolvers, "test-db-scheme")

	// 直接传 nil tracer/meter 验证透传
	_, err := NewFromURL("test-db-scheme://localhost:1234", nil, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotURL != "test-db-scheme://localhost:1234" {
		t.Errorf("rawURL = %q", gotURL)
	}
	if gotTracer != nil {
		t.Errorf("tracer = %v, want nil", gotTracer)
	}
	if gotMeter != nil {
		t.Errorf("meter = %v, want nil", gotMeter)
	}
}

// TestNewFromURL_ResolverError resolver 内部错误透传
func TestNewFromURL_ResolverError(t *testing.T) {
	errSentinel := errors.New("connect failed")
	RegisterResolver("test-db-err", func(string, trace.Tracer, metrics.Meter) (DB, error) {
		return nil, errSentinel
	})
	defer delete(resolvers, "test-db-err")

	_, err := NewFromURL("test-db-err://localhost", nil, nil)
	if !errors.Is(err, errSentinel) {
		t.Errorf("err = %v, want errSentinel", err)
	}
}

// TestRegisterResolver_Idempotent 重复注册保留首次
func TestRegisterResolver_Idempotent(t *testing.T) {
	first := func(string, trace.Tracer, metrics.Meter) (DB, error) { return nil, nil }
	second := func(string, trace.Tracer, metrics.Meter) (DB, error) {
		t.Error("second resolver should not be called")
		return nil, nil
	}

	RegisterResolver("test-db-idemp", first)
	RegisterResolver("test-db-idemp", second)
	defer delete(resolvers, "test-db-idemp")

	if got := resolvers["test-db-idemp"]; got == nil {
		t.Fatal("resolver not registered")
	}
	// 直接调用首次注册的 resolver（不应触发 second）
	_, _ = resolvers["test-db-idemp"]("any", nil, nil)
}

// TestRegisteredResolvers 返回当前已注册的 scheme 集合
func TestRegisteredResolvers(t *testing.T) {
	RegisterResolver("test-db-a", func(string, trace.Tracer, metrics.Meter) (DB, error) { return nil, nil })
	RegisterResolver("test-db-b", func(string, trace.Tracer, metrics.Meter) (DB, error) { return nil, nil })
	defer func() {
		delete(resolvers, "test-db-a")
		delete(resolvers, "test-db-b")
	}()

	set := RegisteredResolvers()
	if _, ok := set["test-db-a"]; !ok {
		t.Error("test-db-a missing")
	}
	if _, ok := set["test-db-b"]; !ok {
		t.Error("test-db-b missing")
	}
}

// TestResolveScheme URL scheme 提取
func TestResolveScheme(t *testing.T) {
	cases := []struct{ in, want string }{
		{"mysql://localhost:3306", "mysql"},
		{"postgres://user@host/db", "postgres"},
		{"no-scheme", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := resolveScheme(tc.in); got != tc.want {
			t.Errorf("resolveScheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
