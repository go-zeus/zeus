package components

import (
	"context"
	"fmt"
	"sync"
)

// Container 组件容器
type Container struct {
	mu      sync.RWMutex
	comps   map[string]Component
	order   []string         // 拓扑排序后的启动顺序
	ctx     *assemblyContext // 装配上下文
	started bool
}

// NewContainer 创建组件容器
func NewContainer() *Container {
	return &Container{
		comps: make(map[string]Component),
		ctx:   newAssemblyContext(),
	}
}

// Register 注册组件
func (c *Container) Register(comp Component) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("components: cannot register after start")
	}

	name := comp.Name()
	if name == "" {
		return fmt.Errorf("components: component name cannot be empty")
	}
	if _, dup := c.comps[name]; dup {
		return fmt.Errorf("components: %q already registered", name)
	}
	c.comps[name] = comp
	c.order = nil
	return nil
}

// Resolve 解析依赖，返回拓扑排序结果
func (c *Container) Resolve() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	order, err := resolve(c.comps)
	if err != nil {
		return err
	}
	c.order = order
	return nil
}

// Start 按拓扑序启动所有组件
func (c *Container) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("components: already started")
	}

	if c.order == nil {
		order, err := resolve(c.comps)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		c.order = order
	}

	// 复制 order 和 comps，释放锁后执行回调
	order := make([]string, len(c.order))
	copy(order, c.order)
	comps := make(map[string]Component, len(c.comps))
	for k, v := range c.comps {
		comps[k] = v
	}
	c.mu.Unlock()

	// 按序调用 Provide（注入用户 context）
	actx := c.ctx.withContext(ctx)
	for _, name := range order {
		comp := comps[name]
		value, err := comp.Provide(actx)
		if err != nil {
			c.stopReverse(ctx, order, comps, name)
			return fmt.Errorf("components: %q provide failed: %w", name, err)
		}
		c.ctx.set(name, value)
	}

	// 按序调用 OnStart（注入用户 context）
	for _, name := range order {
		comp := comps[name]
		lc := comp.Lifecycle()
		if lc.OnStart != nil {
			if err := lc.OnStart(actx); err != nil {
				c.stopReverse(ctx, order, comps, name)
				return fmt.Errorf("components: %q start failed: %w", name, err)
			}
		}
	}

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	return nil
}

// Stop 按逆拓扑序停止所有组件
func (c *Container) Stop(ctx context.Context) error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}

	order := make([]string, len(c.order))
	copy(order, c.order)
	comps := make(map[string]Component, len(c.comps))
	for k, v := range c.comps {
		comps[k] = v
	}
	c.mu.Unlock()

	err := c.stopReverse(ctx, order, comps, "")

	c.mu.Lock()
	c.started = false
	c.mu.Unlock()
	return err
}

// stopReverse 逆序停止组件，传递用户 context 用于超时控制
func (c *Container) stopReverse(ctx context.Context, order []string, comps map[string]Component, stopBefore string) error {
	var firstErr error
	actx := c.ctx.withContext(ctx)
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		if name == stopBefore {
			continue
		}
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			return ctx.Err()
		}
		comp := comps[name]
		lc := comp.Lifecycle()
		if lc.OnStop != nil {
			if err := lc.OnStop(actx); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("components: %q stop failed: %w", name, err)
			}
		}
	}
	return firstErr
}

// Get 按名称获取组件实例
func (c *Container) Get(name string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, err := c.ctx.Get(name)
	if err != nil {
		return nil, false
	}
	return v, true
}

// GetByType 按类型获取组件实例
func GetByType[T any](c *Container) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, err := getByTypeFrom[T](c.ctx)
	if err != nil {
		var zero T
		return zero, false
	}
	return v, true
}
