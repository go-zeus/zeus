package chain

import "net/http"

// Middleware 中间件构造函数
type Middleware func(http.Handler) (http.Handler, error)

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
