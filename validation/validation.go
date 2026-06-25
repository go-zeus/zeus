// Package validation 提供轻量级链式校验工具。
//
// 设计目标：
//   - 零依赖、零反射（避免 validator/v10 的复杂度）
//   - 链式 API：v.Required("name", name).MinLen("name", name, 1).MaxLen("name", name, 100).Err()
//   - 与 errors 包联动：失败时返回带 HTTP code 的 *errors.Error
//   - 可扩展：用户可自定义 Rule 函数挂到 Validator 上
//
// 与 go-playground/validator 的对比：
//
//   - validator：基于 struct tag + 反射，自动化高但调试困难
//   - 本包：纯函数 + 链式调用，每个字段独立校验，调试直观
//
// 使用示例：
//
//	v := validation.New()
//	v.Required("name", name).Email("email", email).Min("age", age, 0)
//	if err := v.Err(); err != nil {
//	    return err // *errors.Error{Reason:"VALIDATION_FAILED", Code:400}
//	}
package validation

import (
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-zeus/zeus/errors"
)

// —— 预定义错误 ——

// ErrValidationFailed 通用校验失败错误（400）
//
// 业务可直接比较：if errors.Is(err, validation.ErrValidationFailed) { ... }
var ErrValidationFailed = errors.New("VALIDATION_FAILED", "validation failed", 400)

// —— Validator 主体 ——

// Validator 校验器，收集所有错误后统一返回
//
// 设计：非 fail-fast，一次性收集所有错误（业务调试更友好）
// 所有 Rule 方法返回 *Validator 自身以支持链式调用
type Validator struct {
	errs []*errors.Error
}

// New 创建空 Validator
func New() *Validator {
	return &Validator{}
}

// Err 返回合并后的错误；无错返回 nil
//
// 多个错误合并：第一个错误作为 outer，message 拼接全部
// causes 完整保留在 Metadata["causes"]
func (v *Validator) Err() error {
	if len(v.errs) == 0 {
		return nil
	}
	if len(v.errs) == 1 {
		return v.errs[0]
	}
	first := *v.errs[0]
	msgs := make([]string, 0, len(v.errs))
	for _, e := range v.errs {
		msgs = append(msgs, e.Message)
	}
	first.Message = strings.Join(msgs, "; ")
	first.Metadata = map[string]any{
		"causes": v.errs,
	}
	return &first
}

// HasErrors 是否已有错误
func (v *Validator) HasErrors() bool {
	return len(v.errs) > 0
}

// Count 已收集错误数
func (v *Validator) Count() int {
	return len(v.errs)
}

// —— 基础类型校验 ——

// Required 字段必填（string 非空 / slice/map 非零长度 / ptr 非 nil）
//
// 适用类型：
//   - string：空字符串失败
//   - slice/array：长度 0 失败
//   - map：长度 0 失败
//   - ptr：nil 失败
//   - 其他：永远通过（数字/bool 没有"空"概念）
func (v *Validator) Required(field string, val any) *Validator {
	if isEmpty(val) {
		v.fail(field, "is required")
	}
	return v
}

// MinLen 最小长度（适用于 string / slice / array / map / channel）
func (v *Validator) MinLen(field string, val any, min int) *Validator {
	n, ok := lengthOf(val)
	if !ok {
		v.fail(field, "has unknown length")
		return v
	}
	if n < min {
		v.fail(field, "length must be >= "+strconv.Itoa(min))
	}
	return v
}

// MaxLen 最大长度
func (v *Validator) MaxLen(field string, val any, max int) *Validator {
	n, ok := lengthOf(val)
	if !ok {
		v.fail(field, "has unknown length")
		return v
	}
	if n > max {
		v.fail(field, "length must be <= "+strconv.Itoa(max))
	}
	return v
}

// Range 长度范围 [min, max]
func (v *Validator) Range(field string, val any, min, max int) *Validator {
	n, ok := lengthOf(val)
	if !ok {
		v.fail(field, "has unknown length")
		return v
	}
	if n < min || n > max {
		v.fail(field, "length must be in ["+strconv.Itoa(min)+","+strconv.Itoa(max)+"]")
	}
	return v
}

// Min 数值最小值（int / int8/16/32/64 / uint 系列 / float32/64）
func (v *Validator) Min(field string, val any, min float64) *Validator {
	n, ok := toFloat(val)
	if !ok {
		v.fail(field, "is not numeric")
		return v
	}
	if n < min {
		v.fail(field, "must be >= "+strconv.FormatFloat(min, 'f', -1, 64))
	}
	return v
}

// Max 数值最大值
func (v *Validator) Max(field string, val any, max float64) *Validator {
	n, ok := toFloat(val)
	if !ok {
		v.fail(field, "is not numeric")
		return v
	}
	if n > max {
		v.fail(field, "must be <= "+strconv.FormatFloat(max, 'f', -1, 64))
	}
	return v
}

// In 值必须在候选集合中（适用于 comparable 类型）
func (v *Validator) In(field string, val any, candidates ...any) *Validator {
	for _, c := range candidates {
		if val == c {
			return v
		}
	}
	v.fail(field, "must be one of the allowed values")
	return v
}

// —— 字符串专属校验 ——

// Email 邮箱格式
func (v *Validator) Email(field, val string) *Validator {
	if val == "" {
		return v // 空字符串交给 Required 管
	}
	if _, err := mail.ParseAddress(val); err != nil {
		v.fail(field, "must be a valid email")
	}
	return v
}

// URL URL 格式
func (v *Validator) URL(field, val string) *Validator {
	if val == "" {
		return v
	}
	u, err := url.Parse(val)
	if err != nil || u.Scheme == "" || u.Host == "" {
		v.fail(field, "must be a valid URL")
	}
	return v
}

// IP IP 地址（v4 或 v6）
func (v *Validator) IP(field, val string) *Validator {
	if val == "" {
		return v
	}
	if net.ParseIP(val) == nil {
		v.fail(field, "must be a valid IP")
	}
	return v
}

// IPv4 IPv4 地址
func (v *Validator) IPv4(field, val string) *Validator {
	if val == "" {
		return v
	}
	ip := net.ParseIP(val)
	if ip == nil || ip.To4() == nil {
		v.fail(field, "must be a valid IPv4")
	}
	return v
}

// IPv6 IPv6 地址
func (v *Validator) IPv6(field, val string) *Validator {
	if val == "" {
		return v
	}
	ip := net.ParseIP(val)
	if ip == nil || ip.To4() != nil {
		v.fail(field, "must be a valid IPv6")
	}
	return v
}

// Match 正则匹配
func (v *Validator) Match(field, val, pattern string) *Validator {
	if val == "" {
		return v
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		v.fail(field, "pattern is invalid")
		return v
	}
	if !re.MatchString(val) {
		v.fail(field, "format is invalid")
	}
	return v
}

// Alpha 纯字母（a-z/A-Z）
func (v *Validator) Alpha(field, val string) *Validator {
	if val == "" {
		return v
	}
	for _, r := range val {
		if !unicode.IsLetter(r) {
			v.fail(field, "must contain only letters")
			return v
		}
	}
	return v
}

// Alphanumeric 字母 + 数字
func (v *Validator) Alphanumeric(field, val string) *Validator {
	if val == "" {
		return v
	}
	for _, r := range val {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			v.fail(field, "must contain only letters and digits")
			return v
		}
	}
	return v
}

// Numeric 纯数字（字符串形式，如 phone "13800138000"）
func (v *Validator) Numeric(field, val string) *Validator {
	if val == "" {
		return v
	}
	for _, r := range val {
		if !unicode.IsDigit(r) {
			v.fail(field, "must contain only digits")
			return v
		}
	}
	return v
}

// NoWhitespace 不能含空白字符
func (v *Validator) NoWhitespace(field, val string) *Validator {
	if val == "" {
		return v
	}
	for _, r := range val {
		if unicode.IsSpace(r) {
			v.fail(field, "must not contain whitespace")
			return v
		}
	}
	return v
}

// —— 高级 API ——

// Rule 注册用户自定义校验规则（泛型顶层函数，因 Validator 不支持泛型方法）。
//
// 参数：
//   - v：目标 Validator（链式调用入口）
//   - field：字段名（仅用于错误信息展示）
//   - val：待校验的值（任意类型 T）
//   - ok：校验回调，返回 true 表示通过，false 表示失败
//   - msg：失败时附加的错误描述（如 "must not contain profanity"）
//
// 返回 v 自身以支持链式调用（与其他 Rule 方法风格一致）。
//
// 用法：
//
//	v := validation.New()
//	validation.Rule(v, "username", username, func(s string) bool {
//	    return !containsProfanity(s)
//	}, "must not contain profanity")
func Rule[T any](v *Validator, field string, val T, ok func(T) bool, msg string) *Validator {
	if !ok(val) {
		v.fail(field, msg)
	}
	return v
}

// Add 直接追加错误（高级用法，跳过检查）
func (v *Validator) Add(field, reason string) *Validator {
	v.fail(field, reason)
	return v
}

// —— 内部辅助 ——

func (v *Validator) fail(field, reason string) {
	v.errs = append(v.errs, errors.New("VALIDATION_FAILED",
		field+" "+reason, 400).WithMetadata(map[string]any{
		"field":  field,
		"reason": reason,
	}))
}

// isEmpty 判断值是否为"空"
//
// 性能：用类型断言（避免反射），ptr nil 用 reflect 兜底
func isEmpty(val any) bool {
	if val == nil {
		return true
	}
	switch x := val.(type) {
	case string:
		return x == ""
	case []byte:
		return len(x) == 0
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, bool:
		return false
	}
	if n, ok := tryLen(val); ok {
		return n == 0
	}
	return isNilPtr(val)
}

// isNilPtr 检测接口包装的 nil ptr（val 是 *T 但值为 nil）
func isNilPtr(val any) bool {
	if val == nil {
		return true
	}
	rv := reflectValue(val)
	return rv.Kind() == reflectPtr && rv.IsNil()
}

// lengthOf 取长度（string / slice / array / map / channel）
func lengthOf(val any) (int, bool) {
	switch x := val.(type) {
	case string:
		return len(x), true
	}
	return tryLen(val)
}

// tryLen 用反射兜底（仅对 slice/array/map/channel/string 有效）
func tryLen(val any) (int, bool) {
	// 这里用类型断言覆盖最常用类型，反射作 fallback
	switch v := val.(type) {
	case []string:
		return len(v), true
	case []int:
		return len(v), true
	case []int32:
		return len(v), true
	case []int64:
		return len(v), true
	case []uint:
		return len(v), true
	case []float32:
		return len(v), true
	case []float64:
		return len(v), true
	case []any:
		return len(v), true
	case map[string]any:
		return len(v), true
	case map[string]string:
		return len(v), true
	}
	// 反射兜底
	return reflectLen(val)
}

// toFloat 任意数值转 float64
func toFloat(val any) (float64, bool) {
	switch x := val.(type) {
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// trimField 用于错误信息字段名规整（避免前后空白）
func trimField(f string) string { return strings.TrimSpace(f) }

var _ = trimField // 保留以便未来扩展
