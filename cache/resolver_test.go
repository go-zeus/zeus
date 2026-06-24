package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestNewFromURL_Empty 空字符串返回 (nil, nil)，调用方自行决定兜底
func TestNewFromURL_Empty(t *testing.T) {
	c, err := NewFromURL("")
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if c != nil {
		t.Errorf("cache = %v, want nil", c)
	}
}

// TestNewFromURL_NoScheme 无 scheme 返回明确错误
func TestNewFromURL_NoScheme(t *testing.T) {
	_, err := NewFromURL("just-a-string")
	if err == nil {
		t.Fatal("expected error for URL without scheme")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("err = %v, want contains 'invalid URL'", err)
	}
}

// TestNewFromURL_UnknownScheme 未知 scheme 返回明确错误（避免静默误导生产）
func TestNewFromURL_UnknownScheme(t *testing.T) {
	_, err := NewFromURL("nonexistent://localhost")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
	if !strings.Contains(err.Error(), "unknown scheme") {
		t.Errorf("err = %v, want contains 'unknown scheme'", err)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("err = %v, want contains scheme name", err)
	}
}

// TestNewFromURL_RegisteredResolver 已注册的 resolver 能被调用
func TestNewFromURL_RegisteredResolver(t *testing.T) {
	called := false
	RegisterResolver("test-cache-scheme", func(rawURL string) (Cache, error) {
		called = true
		if rawURL != "test-cache-scheme://localhost:1234" {
			t.Errorf("rawURL = %q", rawURL)
		}
		return &fakeCacheForResolver{}, nil
	})
	defer func() {
		// 测试隔离：清掉注册项（同进程其他测试不应受影响）
		delete(resolvers, "test-cache-scheme")
	}()

	c, err := NewFromURL("test-cache-scheme://localhost:1234")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Error("resolver not called")
	}
	if c == nil {
		t.Error("cache = nil")
	}
}

// TestNewFromURL_ResolverError resolver 内部错误透传
func TestNewFromURL_ResolverError(t *testing.T) {
	errSentinel := errors.New("dial failed")
	RegisterResolver("test-err-scheme", func(rawURL string) (Cache, error) {
		return nil, errSentinel
	})
	defer delete(resolvers, "test-err-scheme")

	_, err := NewFromURL("test-err-scheme://localhost")
	if !errors.Is(err, errSentinel) {
		t.Errorf("err = %v, want errSentinel", err)
	}
}

// TestRegisterResolver_Idempotent 同一 scheme 重复注册保留首个
func TestRegisterResolver_Idempotent(t *testing.T) {
	first := func(string) (Cache, error) { return &fakeCacheForResolver{}, nil }
	second := func(string) (Cache, error) { return nil, errors.New("should not be called") }

	RegisterResolver("test-idemp", first)
	RegisterResolver("test-idemp", second) // 应被忽略
	defer delete(resolvers, "test-idemp")

	got, ok := resolvers["test-idemp"]
	if !ok {
		t.Fatal("not registered")
	}
	// 调用一次验证仍走 first（second 返回 error，若被覆盖会失败）
	c, err := got("any")
	if err != nil {
		t.Errorf("err = %v, want nil (second resolver should not have replaced first)", err)
	}
	if c == nil {
		t.Error("cache = nil")
	}
}

// TestRegisteredResolvers 返回当前已注册的 scheme 集合
func TestRegisteredResolvers(t *testing.T) {
	RegisterResolver("test-list-a", func(string) (Cache, error) { return nil, nil })
	RegisterResolver("test-list-b", func(string) (Cache, error) { return nil, nil })
	defer func() {
		delete(resolvers, "test-list-a")
		delete(resolvers, "test-list-b")
	}()

	set := RegisteredResolvers()
	if _, ok := set["test-list-a"]; !ok {
		t.Error("test-list-a missing")
	}
	if _, ok := set["test-list-b"]; !ok {
		t.Error("test-list-b missing")
	}
}

// TestResolveScheme 私有 helper：从 URL 提取 scheme
func TestResolveScheme(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"redis://localhost:6379", "redis"},
		{"memory://?cleanup=60s", "memory"},
		{"mysql://user:pass@host:3306/db", "mysql"},
		{"no-scheme", ""},
		{"", ""},
		{"://missing-scheme", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := resolveScheme(tt.in); got != tt.want {
				t.Errorf("resolveScheme(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// fakeCacheForResolver 仅用于 resolver 测试，不实现任何业务
type fakeCacheForResolver struct{}

func (f *fakeCacheForResolver) Get(_ context.Context, _ string) (any, bool) { return nil, false }
func (f *fakeCacheForResolver) Set(_ context.Context, _ string, _ any, _ ...Option) error {
	return nil
}
func (f *fakeCacheForResolver) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeCacheForResolver) Has(_ context.Context, _ string) bool     { return false }
func (f *fakeCacheForResolver) Close() error                             { return nil }
