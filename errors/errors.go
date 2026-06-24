// Package errors 提供 Kratos 风格的业务错误码抽象。
//
// 设计目的：
//   - 统一业务错误格式：reason + message + code + metadata
//   - HTTP / gRPC 双协议自动映射
//   - 与标准 error 接口兼容（可走 errors.Is / errors.As）
//
// 与标准库 errors 的关系：
//   - 标准库 errors：基础 error 包装（New / Wrap / Is / As）
//   - 本包：业务错误码（含 HTTP 状态、可翻译 message、可携带 metadata）
//   - 二者协作：本包的 Error 实现 error 接口，可被 errors.Is 比较
//
// 与 google.golang.org/grpc/status 的关系：
//   - gRPC status：gRPC 协议专用（含 codes.Code + details）
//   - 本包：协议无关，自动映射到 gRPC status 与 HTTP response
//
// 使用建议：
//   - 业务层定义错误码常量：var UserNotFound = errors.New("USER_NOT_FOUND", "user not found", 404)
//   - handler 直接 return err，框架自动按 HTTP code 渲染 JSON
//   - 跨协议调用时 gRPC 自动用 status.Error 转换
package errors

import (
	"fmt"
	"net/http"
)

// Error 业务错误码类型
//
// 字段：
//   - Reason：业务错误码（大写下划线，如 "USER_NOT_FOUND"）
//   - Message：人类可读消息（默认英文，可翻译）
//   - Code：HTTP 状态码映射（200-599）
//   - Metadata：附加数据（如 user_id / trace_id）
type Error struct {
	Reason   string
	Message  string
	Code     int
	Metadata map[string]any
}

// Error 实现 error 接口
func (e *Error) Error() string {
	return fmt.Sprintf("error: reason=%s code=%d message=%s", e.Reason, e.Code, e.Message)
}

// Is 支持 errors.Is 比较：相同 Reason + Code 视为同一错误
//
// 用途：
//
//	if errors.Is(err, UserNotFound) { ... }
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Reason == t.Reason && e.Code == t.Code
}

// As 支持 errors.As 解包到自定义业务错误结构体。
//
// 与标准库 errors.As 协议一致：target 必须是指向实现了 error 接口类型的指针
// （或 *any）。本方法将 e 本身赋值到 target，调用方拿到 *Error 后可自由访问字段。
//
// 用途：
//
//	var target *errors.Error
//	if errors.As(err, &target) {
//	    log.Println(target.Reason, target.Code)
//	}
//
// 注：自定义 error 类型若希望被 As 命中，应自行实现 As 方法或保持 *Error 派生关系。
func (e *Error) As(target any) bool {
	t, ok := target.(**Error)
	if !ok {
		return false
	}
	*t = e
	return true
}

// WithMetadata 返回带 metadata 的副本（不修改原 Error）
//
// 用途：在业务层追加上下文（如 user_id、order_id）
//
// 示例：
//
//	err := UserNotFound.WithMetadata(map[string]any{"user_id": 42})
func (e *Error) WithMetadata(m map[string]any) *Error {
	out := *e // shallow copy
	if e.Metadata != nil {
		out.Metadata = make(map[string]any, len(e.Metadata)+len(m))
		for k, v := range e.Metadata {
			out.Metadata[k] = v
		}
	} else {
		out.Metadata = make(map[string]any, len(m))
	}
	for k, v := range m {
		out.Metadata[k] = v
	}
	return &out
}

// WithMessage 返回带新消息的副本（用于本地化翻译）
func (e *Error) WithMessage(msg string) *Error {
	out := *e
	out.Message = msg
	return &out
}

// GRPCStatus 映射到 gRPC status.Status（与 google.golang.org/grpc/status 兼容）
//
// gRPC code 映射规则（与 grpc-go 一致）：
//   - HTTP 200 → OK
//   - HTTP 400 → InvalidArgument
//   - HTTP 401 → Unauthenticated
//   - HTTP 403 → PermissionDenied
//   - HTTP 404 → NotFound
//   - HTTP 409 → Aborted
//   - HTTP 429 → ResourceExhausted
//   - HTTP 500 → Internal
//   - HTTP 501 → Unimplemented
//   - HTTP 502 → Unavailable
//   - HTTP 503 → Unavailable
//   - HTTP 504 → DeadlineExceeded
//   - 其他 → Unknown
//
// 注：本包不依赖 grpc-go（保持零依赖）。本函数返回纯结构体，
// 业务侧若需 gRPC 集成，可在 transport 层调用 status.New(code, msg) 转换。
func (e *Error) GRPCStatus() (code int, message string) {
	return httpToGRPCCode(e.Code), e.Message
}

// New 创建业务错误码
//
// 参数：
//   - reason：业务错误码字符串（如 "USER_NOT_FOUND"）
//   - message：默认消息
//   - code：HTTP 状态码（200-599）
//
// 推荐用全局变量定义业务错误码常量，再用 WithMessage/WithMetadata 派生
//
// 示例：
//
//	var UserNotFound = errors.New("USER_NOT_FOUND", "user not found", 404)
//	var InvalidParam = errors.New("INVALID_PARAM", "invalid parameter", 400)
func New(reason, message string, code int) *Error {
	return &Error{
		Reason:  reason,
		Message: message,
		Code:    code,
	}
}

// Newf 格式化消息创建错误码
//
// 示例：
//
//	err := errors.Newf("USER_NOT_FOUND", "user %d not found", 42, 404)
func Newf(reason string, code int, format string, args ...any) *Error {
	return &Error{
		Reason:  reason,
		Message: fmt.Sprintf(format, args...),
		Code:    code,
	}
}

// FromError 从任意 error 提取 *Error（若不是 *Error 则包装为 500 Internal）
//
// 用途：handler 层把内部 error 统一转 *Error 后渲染
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return New("INTERNAL", err.Error(), http.StatusInternalServerError)
}

// httpToGRPCCode HTTP 状态码 → gRPC code（数字表示）
//
// gRPC codes 定义参考：google.golang.org/grpc/codes
func httpToGRPCCode(httpCode int) int {
	switch {
	case httpCode >= 200 && httpCode < 300:
		return 0 // OK
	case httpCode == 400:
		return 3 // InvalidArgument
	case httpCode == 401:
		return 16 // Unauthenticated
	case httpCode == 403:
		return 7 // PermissionDenied
	case httpCode == 404:
		return 5 // NotFound
	case httpCode == 409:
		return 10 // Aborted
	case httpCode == 412:
		return 9 // FailedPrecondition
	case httpCode == 429:
		return 8 // ResourceExhausted
	case httpCode == 499:
		return 1 // Canceled
	case httpCode == 500:
		return 13 // Internal
	case httpCode == 501:
		return 12 // Unimplemented
	case httpCode == 502, httpCode == 503:
		return 14 // Unavailable
	case httpCode == 504:
		return 4 // DeadlineExceeded
	default:
		return 2 // Unknown
	}
}

// —— 预定义常用错误码（用户可直接复用或派生） ——

// BadRequest 400 - 客户端请求错误
var BadRequest = New("BAD_REQUEST", "bad request", http.StatusBadRequest)

// Unauthorized 401 - 未认证
var Unauthorized = New("UNAUTHORIZED", "unauthorized", http.StatusUnauthorized)

// Forbidden 403 - 无权限
var Forbidden = New("FORBIDDEN", "forbidden", http.StatusForbidden)

// NotFound 404 - 资源不存在
var NotFound = New("NOT_FOUND", "not found", http.StatusNotFound)

// Conflict 409 - 资源冲突
var Conflict = New("CONFLICT", "conflict", http.StatusConflict)

// Internal 500 - 服务端内部错误
var Internal = New("INTERNAL", "internal server error", http.StatusInternalServerError)

// Unavailable 503 - 服务不可用
var Unavailable = New("UNAVAILABLE", "service unavailable", http.StatusServiceUnavailable)

// Timeout 504 - 网关超时
var Timeout = New("TIMEOUT", "timeout", http.StatusGatewayTimeout)
