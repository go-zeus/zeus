package metadata

import (
	"context"
)

type metadataKey struct{}

// MD 元数据（存在一个上下文中不用加锁）
type MD map[string]string

// Get 获取一个KEY
func (md MD) Get(key string) (string, bool) {
	val, ok := md[key]
	if ok {
		return val, ok
	}
	val, ok = md[key]
	return val, ok
}

// Set 设置一个KEY
func (md MD) Set(key, val string) {
	md[key] = val
}

// Delete 删除一个KEY
func (md MD) Delete(key string) {
	delete(md, key)
}

// Copy 一个新的
func Copy(md MD) MD {
	cmd := make(MD, len(md))
	for k, v := range md {
		cmd[k] = v
	}
	return cmd
}

func (md MD) Equal(o MD) bool {
	if md == nil && o == nil {
		return true
	}
	if md == nil || o == nil {
		return false
	}
	if len(md) != len(o) {
		return false
	}
	for k, v := range md {
		ov, ok := o[k]
		if !ok || v != ov {
			return false
		}
	}
	return true
}

// Delete 从上下文中删除一个KEY
func Delete(ctx context.Context, k string) context.Context {
	return Set(ctx, k, "")
}

// Set 设置到上下文
func Set(ctx context.Context, k, v string) context.Context {
	md, ok := FromContext(ctx)
	if !ok {
		md = make(MD)
	}
	if v == "" {
		delete(md, k)
	} else {
		md[k] = v
	}
	return context.WithValue(ctx, metadataKey{}, md)
}

// Get 从上下文获取一个KEY
func Get(ctx context.Context, key string) (string, bool) {
	md, ok := FromContext(ctx)
	if !ok {
		return "", ok
	}
	val, ok := md[key]
	if ok {
		return val, ok
	}
	val, ok = md[key]

	return val, ok
}

// FromContext 从上下文获取所有
func FromContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(metadataKey{}).(MD)
	if !ok {
		return nil, ok
	}
	newMD := make(MD, len(md))
	for k, v := range md {
		newMD[k] = v
	}

	return newMD, ok
}

// NewContext 创建一个上下文的元数据
func NewContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, metadataKey{}, md)
}

// MergeContext 合并到一个上下文中（overwrite 是否覆盖）
func MergeContext(ctx context.Context, patchMd MD, overwrite bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	md, _ := ctx.Value(metadataKey{}).(MD)
	cmd := make(MD, len(md))
	for k, v := range md {
		cmd[k] = v
	}
	for k, v := range patchMd {
		if _, ok := cmd[k]; ok && !overwrite {
			// skip
		} else if v != "" {
			cmd[k] = v
		} else {
			delete(cmd, k) //设置一个空值表示删除
		}
	}
	return context.WithValue(ctx, metadataKey{}, cmd)
}
