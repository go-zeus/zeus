package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestValidate_ValidSpec 合法 Spec 通过
func TestValidate_ValidSpec(t *testing.T) {
	cases := []Spec{
		{Name: "j1", Every: 30 * time.Second, Handler: func(context.Context) error { return nil }},
		{Name: "j2", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return nil }},
		{Name: "j3", Every: 1 * time.Second, Schedule: "ignored", Handler: func(context.Context) error { return nil }, Timeout: 5 * time.Second},
	}
	for i, s := range cases {
		if err := s.Validate(); err != nil {
			t.Errorf("case %d: Validate err: %v", i, err)
		}
	}
}

// TestValidate_MissingName Name 必填
func TestValidate_MissingName(t *testing.T) {
	s := Spec{Every: 1 * time.Second, Handler: func(context.Context) error { return nil }}
	err := s.Validate()
	if err == nil {
		t.Error("Validate should fail for missing Name")
	}
}

// TestValidate_MissingHandler Handler 必填
func TestValidate_MissingHandler(t *testing.T) {
	s := Spec{Name: "j", Every: 1 * time.Second}
	err := s.Validate()
	if err == nil {
		t.Error("Validate should fail for missing Handler")
	}
}

// TestValidate_MissingScheduleOrEvery Every 和 Schedule 至少一个
func TestValidate_MissingScheduleOrEvery(t *testing.T) {
	s := Spec{Name: "j", Handler: func(context.Context) error { return nil }}
	err := s.Validate()
	if err == nil {
		t.Error("Validate should fail when Every=0 and Schedule empty")
	}
}

// TestValidate_NegativeEvery 负 Every 视为缺失
func TestValidate_NegativeEvery(t *testing.T) {
	s := Spec{Name: "j", Every: -1 * time.Second, Handler: func(context.Context) error { return nil }}
	err := s.Validate()
	if err == nil {
		t.Error("Validate should fail for negative Every")
	}
}

// TestValidate_ZeroEveryWithSchedule Every=0 但 Schedule 非空：通过
func TestValidate_ZeroEveryWithSchedule(t *testing.T) {
	s := Spec{Name: "j", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return nil }}
	if err := s.Validate(); err != nil {
		t.Errorf("Validate err: %v", err)
	}
}

// TestErrorHandler_NilSafety ErrorHandler 类型可以正确调用
func TestErrorHandler_NilSafety(t *testing.T) {
	// 类型本身只是 func，无内部状态可测；验证可以被调用
	called := false
	var h ErrorHandler = func(name string, err error) {
		called = true
		if name != "j" {
			t.Errorf("name = %q", name)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	}
	h("j", context.Canceled)
	if !called {
		t.Error("ErrorHandler not called")
	}
}

// TestHandler_NilSafety Handler 类型可正确调用
func TestHandler_NilSafety(t *testing.T) {
	var h Handler = func(ctx context.Context) error {
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h(ctx); err == nil {
		t.Error("Handler should return ctx.Err()")
	}
}

// TestSpec_ZeroValue 零值 Validate 失败
func TestSpec_ZeroValue(t *testing.T) {
	var s Spec
	if err := s.Validate(); err == nil {
		t.Error("zero Spec should fail Validate")
	}
}
