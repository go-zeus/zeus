package recovery

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

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
			// panic value 清理：去除换行/控制字符，防止拼入错误消息后被日志当作独立行（日志注入）
			// 保留 stack trace 原样（debug.Stack 本身多行，是排查依据）
			recStr := sanitizePanicValue(fmt.Sprint(rec))
			// 在错误信息中包含请求上下文
			if req != nil {
				err = fmt.Errorf("middleware/recovery: panic recovered: %s, method=%s, path=%s\n%s", recStr, req.Method(), req.Path(), debug.Stack())
			} else {
				err = fmt.Errorf("middleware/recovery: panic recovered: %s\n%s", recStr, debug.Stack())
			}
		}
	}()
	return handler(ctx, req)
}

func (r *recoveryInterceptor) Name() string {
	return "recovery"
}

// sanitizePanicValue 清理 panic value 字符串中的换行/控制字符，
// 防止日志注入（伪造日志行 / 终端转义序列攻击）。
//
// 保留可见 ASCII 与常见空白（空格/Tab）；\n \r 替换为可视化形式，
// 其他控制字符（< 0x20 或 0x7f）替换为 "?"。
func sanitizePanicValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteString("\\n")
		case r == '\r':
			b.WriteString("\\r")
		case r == '\t':
			b.WriteString("\\t")
		case r < 0x20 || r == 0x7f:
			b.WriteString("?")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
