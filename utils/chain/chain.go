package chain

import "net/http"

// Middleware 中间件构造函数
type Middleware func(http.Handler) (http.Handler, error)

// Pure 无错误版本的中间件类型（对齐 alice / chi / gorilla 主流签名）
//
// 用途：让大量已有的 `func(http.Handler) http.Handler` 中间件（如 gorilla/handlers）
// 无需包装即可加入 Chain
//
// 与 Middleware 的关系：
//   - Middleware 返回 (handler, error) —— 支持构造期校验（如配置错误）
//   - Pure 返回 handler —— 适合纯运行时逻辑（无错误路径）
//
// 转换：用 FromPure 把 Pure 包装为 Middleware
type Pure func(http.Handler) http.Handler

// FromPure 把 Pure 转换为 Middleware（适配 alice 风格中间件）
//
// 示例：
//
//	import "github.com/gorilla/handlers"
//	ch := chain.New(chain.FromPure(handlers.CompressHandler))
func FromPure(p Pure) Middleware {
	return func(h http.Handler) (http.Handler, error) {
		return p(h), nil
	}
}

// Chain 处理程序的构造函数
type Chain struct {
	middlewares []Middleware
}

// New 创建中间件链
func New(middlewares ...Middleware) Chain {
	return Chain{middlewares: middlewares}
}

// Then 返回最终的 http.Handler. New(m1, m2, m3).Then(h) -> m1(m2(m3(h)))
func (c Chain) Then(h http.Handler) (http.Handler, error) {
	if h == nil {
		h = http.DefaultServeMux
	}

	for i := range c.middlewares {
		handler, err := c.middlewares[len(c.middlewares)-1-i](h)
		if err != nil {
			return nil, err
		}
		h = handler
	}

	return h, nil
}

// ThenFunc 与then相同，但参数为 http.HandlerFunc
func (c Chain) ThenFunc(fn http.HandlerFunc) (http.Handler, error) {
	if fn == nil {
		return c.Then(nil)
	}
	return c.Then(fn)
}

// Append 扩展链
//
//	stdChain := alice.New(m1, m2)  m1 -> m2
//	extChain := stdChain.Append(m3, m4)  m1 -> m2 -> m3 -> m4
func (c Chain) Append(middlewares ...Middleware) Chain {
	newMiddles := make([]Middleware, 0, len(c.middlewares)+len(middlewares))
	newMiddles = append(newMiddles, c.middlewares...)
	newMiddles = append(newMiddles, middlewares...)

	return Chain{newMiddles}
}

// Extend 合并链
//
//	stdChain := alice.New(m1, m2)  m1 -> m2
//	ext1Chain := alice.New(m3, m4)  m3 -> m4
//	ext2Chain := stdChain.Extend(ext1Chain)  m1 -> m2 -> m3 -> m4
func (c Chain) Extend(chain Chain) Chain {
	return c.Append(chain.middlewares...)
}

// Must 把 (handler, error) 返回值中的 error 转 panic
//
// 用途：当中间件构造期错误"理论上不可能发生"时，简化 init 代码
//
// 示例：
//
//	// 全局变量初始化
//	var handler = chain.Must(chain.New(mw1, mw2).Then(app))
func Must(h http.Handler, err error) http.Handler {
	if err != nil {
		panic(err)
	}
	return h
}

// MustThen 等价于 Must(c.Then(h))，链式调用更简洁
//
// 示例：
//
//	handler := chain.New(mw1, mw2).MustThen(app)
func (c Chain) MustThen(h http.Handler) http.Handler {
	return Must(c.Then(h))
}
