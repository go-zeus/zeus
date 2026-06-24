// L1 示例：5 行 Hello World
//
// 零配置启动一个微服务，内置：
//   - slog 日志（stdout）
//   - requestid + accesslog + recovery 中间件
//   - memory 注册中心 + 自动注册
//   - 优雅关闭（SIGINT/SIGTERM，10s 超时）
//
// 启动：
//
//	go run .
//
// 测试：
//
//	curl http://localhost:8080/hi
//	curl -H "X-Request-ID: my-trace" http://localhost:8080/hi   # 自定义 request id
//	kill -INT <pid>                                                # 触发优雅关闭
package main

import (
	"net/http"

	"github.com/go-zeus/zeus/app"
)

func main() {
	app.Run(&app.Config{Port: 8080}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from zeus L1"))
	}))
}

// 预期输出（日志）：
//
//	... INFO req GET /hi status=200 duration=... ip=127.0.0.1:... request_id=...
//	... INFO service zeus-service registered: ...
