package timeout

import (
	"context"
	"fmt"
	"time"

	"github.com/go-zeus/zeus/middleware"
)

// 编译期检查 timeoutInterceptor 实现了 middleware.Interceptor 接口
var _ middleware.Interceptor = (*timeoutInterceptor)(nil)

type timeoutInterceptor struct {
	timeout time.Duration
}

// New 创建超时中间件
func New(timeout time.Duration) middleware.Interceptor {
	return &timeoutInterceptor{timeout: timeout}
}

func (t *timeoutInterceptor) Intercept(ctx context.Context, req middleware.Request, handler middleware.Handler) (middleware.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	resp, err := handler(ctx, req)
	if ctx.Err() == context.DeadlineExceeded {
		return resp, fmt.Errorf("middleware/timeout: request timed out after %v", t.timeout)
	}
	return resp, err
}

func (t *timeoutInterceptor) Name() string {
	return "timeout"
}
