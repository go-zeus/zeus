// Package validation 内部反射兜底（仅用于类型断言未覆盖的场景）
package validation

import "reflect"

// reflectLen 通过反射取长度
//
// 适用：slice / array / map / channel / string
// 不适用：int / float / bool / struct / ptr（返回 0, false）
func reflectLen(val any) (int, bool) {
	if val == nil {
		return 0, false
	}
	v := reflectValue(val)
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.String:
		return v.Len(), true
	}
	return 0, false
}

// reflectValue 安全取 reflect.Value
//
// 注意：val 是 any（接口），reflect.ValueOf 已自动 unwrap 一层接口
// 对于 untyped nil（接口本身为 nil）返回 zero Value
func reflectValue(val any) reflect.Value {
	return reflect.ValueOf(val)
}

// reflectPtr 常量别名，便于 isEmpty 使用
const reflectPtr = reflect.Ptr
