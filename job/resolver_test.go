package job

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestResolveScheme 基础 URL 提取 scheme 正确
func TestResolveScheme(t *testing.T) {
	cases := map[string]string{
		"interval://":          "interval",
		"cron://?seconds=true": "cron",
		"cron://loc=UTC":       "cron",
		"no-scheme":            "",
		"":                     "",
		"://empty":             "",
	}
	for in, want := range cases {
		if got := resolveScheme(in); got != want {
			t.Errorf("resolveScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRegisterResolver_Idempotent 同一 scheme 重复注册保留首个
func TestRegisterResolver_Idempotent(t *testing.T) {
	first := func(string) (Scheduler, error) { return nil, nil }
	second := func(string) (Scheduler, error) { return nil, errors.New("should not be called") }
	RegisterResolver("test-idemp-job", first)
	RegisterResolver("test-idemp-job", second)

	r := resolvers["test-idemp-job"]
	if _, err := r(""); err != nil {
		t.Errorf("expected first resolver to win, got err = %v", err)
	}
}

// TestRegisterResolver_EmptyIgnored 空 scheme / nil resolver 不注册
func TestRegisterResolver_EmptyIgnored(t *testing.T) {
	before := len(resolvers)
	RegisterResolver("", func(string) (Scheduler, error) { return nil, nil })
	RegisterResolver("test-nil", nil)
	if len(resolvers) != before {
		t.Errorf("expected resolvers map unchanged, got size %d (before %d)", len(resolvers), before)
	}
}

// TestRegisteredResolvers 返回当前注册的全部 scheme
func TestRegisteredResolvers(t *testing.T) {
	RegisterResolver("test-list-job-a", func(string) (Scheduler, error) { return nil, nil })
	RegisterResolver("test-list-job-b", func(string) (Scheduler, error) { return nil, nil })

	out := RegisteredResolvers()
	if _, ok := out["test-list-job-a"]; !ok {
		t.Error("missing test-list-job-a")
	}
	if _, ok := out["test-list-job-b"]; !ok {
		t.Error("missing test-list-job-b")
	}
}

// TestNewSchedulerFromURL_Empty 空字符串返回 (nil, nil)
func TestNewSchedulerFromURL_Empty(t *testing.T) {
	s, err := NewSchedulerFromURL("")
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if s != nil {
		t.Errorf("scheduler = %v, want nil", s)
	}
}

// TestNewSchedulerFromURL_NoScheme 无 scheme 报错
func TestNewSchedulerFromURL_NoScheme(t *testing.T) {
	_, err := NewSchedulerFromURL("just-a-string")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("err = %v, want contains 'invalid URL'", err)
	}
}

// TestNewSchedulerFromURL_UnknownScheme 未注册 scheme 报错并给出 import 提示
func TestNewSchedulerFromURL_UnknownScheme(t *testing.T) {
	_, err := NewSchedulerFromURL("totally-unknown://")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
	if !strings.Contains(err.Error(), "unknown scheme") {
		t.Errorf("err = %v, want contains 'unknown scheme'", err)
	}
	if !strings.Contains(err.Error(), "import") {
		t.Errorf("err = %v, want contains 'import' hint", err)
	}
}

// TestNewSchedulerFromURL_Registered 注册的 resolver 被正确调用
func TestNewSchedulerFromURL_Registered(t *testing.T) {
	sentinel := &fakeScheduler{}
	RegisterResolver("test-valid-job", func(rawURL string) (Scheduler, error) {
		if rawURL != "test-valid-job://?foo=bar" {
			return nil, errors.New("URL mismatch")
		}
		return sentinel, nil
	})

	s, err := NewSchedulerFromURL("test-valid-job://?foo=bar")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s != sentinel {
		t.Error("returned scheduler not the sentinel instance")
	}
}

// fakeScheduler 仅用于测试（不参与实际调度）
type fakeScheduler struct{}

func (*fakeScheduler) Register(...Spec) error      { return nil }
func (*fakeScheduler) Start(context.Context) error { return nil }
func (*fakeScheduler) Stop(context.Context) error  { return nil }
