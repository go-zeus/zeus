package components

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// Context 装配上下文，组件通过它获取其他组件的实例
// 同时嵌入 context.Context，使生命周期钩子（OnStart/OnStop）可感知取消/超时
type Context interface {
	context.Context

	// Get 按名称获取依赖实例
	Get(name string) (any, error)
}

// Type 按类型从装配上下文获取依赖实例。
//
// Go 接口不支持泛型方法，因此提供顶层泛型函数。
// 命名遵循 Go 惯例：导出函数不加 Get 前缀（参考 http.Get、os.Stat）。
func Type[T any](ctx Context) (T, error) {
	ac, ok := ctx.(*assemblyContext)
	if !ok {
		var zero T
		return zero, fmt.Errorf("components: unsupported context type %T", ctx)
	}
	return getByTypeFrom[T](ac)
}

// AllByType 从装配上下文按类型批量获取所有实例，按注册（拓扑序 Provide）顺序返回。
//
// 用途：当多个组件 Provide 相同接口类型（例如多个 MiddlewareComponent 都返回
// middleware.Interceptor）时，Type 只能拿到一个，AllByType 拿到全部。
//
// 注意：调用时机必须在所有相关组件 Provide 完成之后（例如 OnStart），
// 否则可能拿不全。
func AllByType[T any](ctx Context) ([]T, error) {
	ac, ok := ctx.(*assemblyContext)
	if !ok {
		return nil, fmt.Errorf("components: unsupported context type %T", ctx)
	}
	return getAllByTypeFrom[T](ac)
}

// assemblyContext 装配上下文实现
// 持有当前 stdlib context，由 Container.Start/Stop 注入（用于 Provide/OnStart/OnStop 期间感知取消/超时）
//
// order 字段使用 *[]string 共享指针：所有通过 withContext 派生的 ctx 看到的都是同一份
// 注册顺序快照（避免 Provide 阶段 append 后派生 ctx 看到旧 slice header 的 bug）。
type assemblyContext struct {
	context.Context // 当前 stdlib context，由 withContext 临时替换

	mu        sync.RWMutex
	providers map[string]any
	order     *[]string // 按 set 调用顺序记录 name（所有派生 ctx 共享同一指针）
	byType    map[reflect.Type]any
}

func newAssemblyContext() *assemblyContext {
	order := make([]string, 0)
	return &assemblyContext{
		Context:   context.Background(),
		providers: make(map[string]any),
		order:     &order,
		byType:    make(map[reflect.Type]any),
	}
}

// withContext 返回一个临时切换 stdlib context 的视图（不改原 ctx）
// Container 在调用 Provide/OnStart/OnStop 时使用它注入用户的 context
func (c *assemblyContext) withContext(ctx context.Context) *assemblyContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &assemblyContext{
		Context:   ctx,
		providers: c.providers,
		order:     c.order, // 共享指针：派生 ctx 看到 set() 后的最新 order
		byType:    c.byType,
	}
}

func (c *assemblyContext) Get(name string) (any, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.providers[name]
	if !ok {
		return nil, fmt.Errorf("components: %q not found", name)
	}
	return v, nil
}

func (c *assemblyContext) set(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.providers[name]; !exists {
		// 通过指针追加：所有派生 assemblyContext 都能看到新 name
		*c.order = append(*c.order, name)
	}
	c.providers[name] = value
	if value != nil {
		rt := reflect.TypeOf(value)
		c.byType[rt] = value
	}
}

// getByTypeFrom 从 assemblyContext 按类型获取实例
// 查找顺序：
//  1. 精确类型匹配（最常见的具体类型查找）
//  2. 接口可赋值匹配（按接口类型查找时，扫描所有已注册实例）
func getByTypeFrom[T any](ac *assemblyContext) (T, error) {
	var zero T
	rt := reflect.TypeOf((*T)(nil)).Elem()

	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// 1. 精确类型匹配
	if v, ok := ac.byType[rt]; ok {
		typed, ok := v.(T)
		if ok {
			return typed, nil
		}
	}

	// 2. 接口可赋值匹配（T 是接口，扫描所有已注册实例）
	if rt.Kind() == reflect.Interface {
		for _, name := range *ac.order {
			v, ok := ac.providers[name]
			if !ok || v == nil {
				continue
			}
			if reflect.TypeOf(v).Implements(rt) {
				if typed, ok := v.(T); ok {
					return typed, nil
				}
			}
		}
	}

	return zero, fmt.Errorf("components: type %v not found", rt)
}

// getAllByTypeFrom 从 assemblyContext 按类型批量获取所有实例（按 set 顺序遍历）。
//
// 典型场景：多个组件 Provide 相同类型实例（典型如多个 MiddlewareComponent
// 都返回 middleware.Interceptor）。GetByType 只返回首个匹配，本函数返回全部。
func getAllByTypeFrom[T any](ac *assemblyContext) ([]T, error) {
	rt := reflect.TypeOf((*T)(nil)).Elem()

	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var result []T
	for _, name := range *ac.order {
		v, ok := ac.providers[name]
		if !ok || v == nil {
			continue
		}
		// 精确类型 / 接口实现均可
		if typed, ok := v.(T); ok {
			result = append(result, typed)
			continue
		}
		if rt.Kind() == reflect.Interface && reflect.TypeOf(v).Implements(rt) {
			if typed, ok := v.(T); ok {
				result = append(result, typed)
			}
		}
	}
	return result, nil
}
