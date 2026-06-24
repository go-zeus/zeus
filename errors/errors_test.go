package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"
)

// —— New / Error / Error() ——

func TestNew_Fields(t *testing.T) {
	e := New("USER_NOT_FOUND", "user not found", 404)
	if e.Reason != "USER_NOT_FOUND" {
		t.Errorf("Reason = %q", e.Reason)
	}
	if e.Message != "user not found" {
		t.Errorf("Message = %q", e.Message)
	}
	if e.Code != 404 {
		t.Errorf("Code = %d", e.Code)
	}
}

func TestError_ErrorString(t *testing.T) {
	e := New("BAD_REQUEST", "missing field", 400)
	s := e.Error()
	if s == "" {
		t.Error("Error() should not be empty")
	}
	if !contains(s, "BAD_REQUEST") {
		t.Errorf("Error() should contain reason: %q", s)
	}
	if !contains(s, "missing field") {
		t.Errorf("Error() should contain message: %q", s)
	}
	if !contains(s, "400") {
		t.Errorf("Error() should contain code: %q", s)
	}
}

func TestNewf_FormattedMessage(t *testing.T) {
	e := Newf("INVALID_PARAM", 400, "field %s is invalid: %d", "age", -1)
	if e.Message != "field age is invalid: -1" {
		t.Errorf("Message = %q", e.Message)
	}
}

// —— Is ——

func TestIs_SameReasonAndCode(t *testing.T) {
	a := New("USER_NOT_FOUND", "msg1", 404)
	b := New("USER_NOT_FOUND", "msg2", 404) // 不同 message 但同 reason + code
	if !stderrors.Is(a, b) {
		t.Error("errors.Is should match on same reason + code")
	}
}

func TestIs_DifferentReason(t *testing.T) {
	a := New("USER_NOT_FOUND", "", 404)
	b := New("ORDER_NOT_FOUND", "", 404)
	if stderrors.Is(a, b) {
		t.Error("errors.Is should NOT match on different reason")
	}
}

func TestIs_DifferentCode(t *testing.T) {
	a := New("USER_NOT_FOUND", "", 404)
	b := New("USER_NOT_FOUND", "", 500)
	if stderrors.Is(a, b) {
		t.Error("errors.Is should NOT match on different code")
	}
}

func TestIs_NonErrorTarget(t *testing.T) {
	e := New("X", "y", 500)
	other := stderrors.New("plain error")
	if stderrors.Is(e, other) {
		t.Error("Is should return false for non-*Error target")
	}
	if stderrors.Is(other, e) {
		t.Error("Is(*Error, plain error) should be false")
	}
}

// —— As ——

func TestAs_UnwrapsToError(t *testing.T) {
	e := New("USER_NOT_FOUND", "user not found", 404)
	var target *Error
	if !stderrors.As(e, &target) {
		t.Fatal("errors.As should succeed for *Error target")
	}
	if target.Reason != "USER_NOT_FOUND" {
		t.Errorf("Reason = %q", target.Reason)
	}
	if target.Code != 404 {
		t.Errorf("Code = %d", target.Code)
	}
}

func TestAs_FromWrappedError(t *testing.T) {
	// fmt.Errorf 包装一层后，errors.As 仍应命中 *Error
	original := New("CONFLICT", "dup", 409)
	wrapped := fmt.Errorf("wrap: %w", original)
	var target *Error
	if !stderrors.As(wrapped, &target) {
		t.Fatal("errors.As should unwrap to *Error")
	}
	if target.Reason != "CONFLICT" {
		t.Errorf("Reason = %q", target.Reason)
	}
}

// myErr 用于 TestAs_NonErrorTarget：实现 error 接口但与 *Error 无关
type myErr struct{ msg string }

func (m *myErr) Error() string { return m.msg }

func TestAs_NonErrorTarget(t *testing.T) {
	e := New("X", "y", 500)
	var target *myErr
	// stderrors.As 会调用 e.As(&target)，e.As 检测到 target 不是 **Error 时返回 false
	if stderrors.As(e, &target) {
		t.Error("As should return false for non-*Error target type")
	}
	if target != nil {
		t.Error("target should remain nil when As returns false")
	}
}

// —— WithMetadata ——

func TestWithMetadata_AddsMetadata(t *testing.T) {
	e := New("X", "y", 400)
	e2 := e.WithMetadata(map[string]any{"user_id": 42})
	if e2.Metadata["user_id"] != 42 {
		t.Errorf("Metadata[user_id] = %v", e2.Metadata["user_id"])
	}
}

func TestWithMetadata_DoesNotMutateOriginal(t *testing.T) {
	e := New("X", "y", 400)
	_ = e.WithMetadata(map[string]any{"k": "v"})
	if e.Metadata != nil {
		t.Error("WithMetadata should not mutate original")
	}
}

func TestWithMetadata_MergesMetadata(t *testing.T) {
	e := New("X", "y", 400).
		WithMetadata(map[string]any{"a": 1})
	e2 := e.WithMetadata(map[string]any{"b": 2})
	if e2.Metadata["a"] != 1 || e2.Metadata["b"] != 2 {
		t.Errorf("merged metadata = %v", e2.Metadata)
	}
}

func TestWithMessage(t *testing.T) {
	e := New("X", "english", 400).WithMessage("中文")
	if e.Message != "中文" {
		t.Errorf("Message = %q", e.Message)
	}
}

// —— FromError ——

func TestFromError_NilReturnsNil(t *testing.T) {
	if FromError(nil) != nil {
		t.Error("FromError(nil) should return nil")
	}
}

func TestFromError_AlreadyIsError(t *testing.T) {
	original := New("X", "y", 400)
	if FromError(original) != original {
		t.Error("FromError(*Error) should return same instance")
	}
}

func TestFromError_PlainErrorWrapsAs500(t *testing.T) {
	plain := stderrors.New("disk failure")
	e := FromError(plain)
	if e.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want 500", e.Code)
	}
	if e.Reason != "INTERNAL" {
		t.Errorf("Reason = %q, want INTERNAL", e.Reason)
	}
	if e.Message != "disk failure" {
		t.Errorf("Message = %q", e.Message)
	}
}

// —— GRPCStatus ——

func TestGRPCStatus_HTTPToGRPCMapping(t *testing.T) {
	cases := []struct {
		httpCode int
		wantCode int
	}{
		{200, 0},  // OK
		{400, 3},  // InvalidArgument
		{401, 16}, // Unauthenticated
		{403, 7},  // PermissionDenied
		{404, 5},  // NotFound
		{409, 10}, // Aborted
		{429, 8},  // ResourceExhausted
		{500, 13}, // Internal
		{501, 12}, // Unimplemented
		{502, 14}, // Unavailable
		{503, 14}, // Unavailable
		{504, 4},  // DeadlineExceeded
		{418, 2},  // Unknown (teapot)
	}
	for _, c := range cases {
		e := New("X", "y", c.httpCode)
		gotCode, _ := e.GRPCStatus()
		if gotCode != c.wantCode {
			t.Errorf("HTTP %d → gRPC code %d, want %d", c.httpCode, gotCode, c.wantCode)
		}
	}
}

// —— 预定义错误码 ——

func TestPredefinedErrors(t *testing.T) {
	if BadRequest.Code != 400 {
		t.Errorf("BadRequest.Code = %d", BadRequest.Code)
	}
	if Unauthorized.Code != 401 {
		t.Errorf("Unauthorized.Code = %d", Unauthorized.Code)
	}
	if NotFound.Code != 404 {
		t.Errorf("NotFound.Code = %d", NotFound.Code)
	}
	if Internal.Code != 500 {
		t.Errorf("Internal.Code = %d", Internal.Code)
	}
}

func TestPredefined_CanBeUsedWithIs(t *testing.T) {
	// 业务层用预定义错误码 + errors.Is 判定
	err := NotFound.WithMetadata(map[string]any{"id": 42})
	if !stderrors.Is(err, NotFound) {
		t.Error("derived error should match predefined via Is")
	}
}

// —— 综合场景 ——

func TestComposite_HandlerPattern(t *testing.T) {
	// 模拟业务层返回错误，handler 层统一渲染
	findUser := func(id int) error {
		if id == 0 {
			return NotFound.WithMetadata(map[string]any{"id": id})
		}
		return nil
	}

	// 业务调用
	err := findUser(0)
	if err == nil {
		t.Fatal("expected error")
	}

	// handler 层
	e := FromError(err)
	if e.Code != 404 {
		t.Errorf("HTTP code = %d, want 404", e.Code)
	}
	if !stderrors.Is(err, NotFound) {
		t.Error("should match NotFound")
	}
	if e.Metadata["id"] != 0 {
		t.Errorf("metadata = %v", e.Metadata)
	}
}

// —— 辅助函数 ——

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// —— 占位：避免 import 未使用 ——

var _ = fmt.Sprintf
