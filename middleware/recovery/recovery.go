package recovery

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/go-zeus/zeus/middleware"
)

// 编译期检查 recoveryInterceptor 实现了 middleware.Interceptor 接口
var _ middleware.Interceptor = (*recoveryInterceptor)(nil)

type recoveryInterceptor struct{}

// New 创建 recovery 中间件
func New() middleware.Interceptor {
	return &recoveryInterceptor{}
}

func (r *recoveryInterceptor) Intercept(ctx context.Context, req middleware.Request, handler middleware.Handler) (resp middleware.Response, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			// 在错误信息中包含请求上下文
			if req != nil {
				err = fmt.Errorf("middleware/recovery: panic recovered: %v, method=%s, path=%s\n%s", rec, req.Method(), req.Path(), debug.Stack())
			} else {
				err = fmt.Errorf("middleware/recovery: panic recovered: %v\n%s", rec, debug.Stack())
			}
		}
	}()
	return handler(ctx, req)
}

func (r *recoveryInterceptor) Name() string {
	return "recovery"
}
