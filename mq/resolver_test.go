package mq

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeBroker 仅用于 resolver 测试
type fakeBroker struct{}

func (fakeBroker) Publish(context.Context, string, *Message) error  { return nil }
func (fakeBroker) Subscribe(context.Context, string, Handler) error { return nil }
func (fakeBroker) Close() error                                     { return nil }

// TestNewBrokerFromURL_Empty 空字符串返回 (nil, nil)
func TestNewBrokerFromURL_Empty(t *testing.T) {
	b, err := NewBrokerFromURL("")
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if b != nil {
		t.Errorf("broker = %v, want nil", b)
	}
}

// TestNewBrokerFromURL_NoScheme 无 scheme 返回错误
func TestNewBrokerFromURL_NoScheme(t *testing.T) {
	_, err := NewBrokerFromURL("no-scheme-here")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("err = %v", err)
	}
}

// TestNewBrokerFromURL_UnknownScheme 未知 scheme 返回错误
func TestNewBrokerFromURL_UnknownScheme(t *testing.T) {
	_, err := NewBrokerFromURL("nonexistent://localhost")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown scheme") {
		t.Errorf("err = %v", err)
	}
}

// TestNewBrokerFromURL_Registered 已注册的 resolver 能被调用
func TestNewBrokerFromURL_Registered(t *testing.T) {
	called := false
	RegisterResolver("test-mq-scheme", func(rawURL string) (Broker, error) {
		called = true
		if rawURL != "test-mq-scheme://host:1234" {
			t.Errorf("rawURL = %q", rawURL)
		}
		return &fakeBroker{}, nil
	})
	defer delete(resolvers, "test-mq-scheme")

	b, err := NewBrokerFromURL("test-mq-scheme://host:1234")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Error("resolver not called")
	}
	if b == nil {
		t.Error("broker is nil")
	}
}

// TestNewBrokerFromURL_ResolverError resolver 错误透传
func TestNewBrokerFromURL_ResolverError(t *testing.T) {
	sentinel := errors.New("dial failed")
	RegisterResolver("test-mq-err", func(string) (Broker, error) { return nil, sentinel })
	defer delete(resolvers, "test-mq-err")

	_, err := NewBrokerFromURL("test-mq-err://localhost")
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
}

// TestRegisterResolver_Idempotent 重复注册保留首次
func TestRegisterResolver_Idempotent(t *testing.T) {
	first := func(string) (Broker, error) { return &fakeBroker{}, nil }
	second := func(string) (Broker, error) {
		t.Error("second should not be called")
		return nil, nil
	}

	RegisterResolver("test-mq-idemp", first)
	RegisterResolver("test-mq-idemp", second)
	defer delete(resolvers, "test-mq-idemp")

	// 调用一次验证仍走 first
	got, err := resolvers["test-mq-idemp"]("any")
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if got == nil {
		t.Error("broker is nil")
	}
}

// TestRegisteredResolvers 返回当前已注册的 scheme 集合
func TestRegisteredResolvers(t *testing.T) {
	RegisterResolver("test-mq-a", func(string) (Broker, error) { return nil, nil })
	RegisterResolver("test-mq-b", func(string) (Broker, error) { return nil, nil })
	defer func() {
		delete(resolvers, "test-mq-a")
		delete(resolvers, "test-mq-b")
	}()

	set := RegisteredResolvers()
	if _, ok := set["test-mq-a"]; !ok {
		t.Error("test-mq-a missing")
	}
	if _, ok := set["test-mq-b"]; !ok {
		t.Error("test-mq-b missing")
	}
}

// TestResolveScheme URL scheme 提取
func TestResolveScheme(t *testing.T) {
	cases := []struct{ in, want string }{
		{"memory://", "memory"},
		{"kafka://host:9092", "kafka"},
		{"no-scheme", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := resolveScheme(tc.in); got != tc.want {
			t.Errorf("resolveScheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
