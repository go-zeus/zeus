package main

import (
	"net/http"
	"time"

	"github.com/go-zeus/zeus/components"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/registry/memory"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

func main() {
	// 构造 HTTP server，注册业务路由
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from zeus"))
	})

	// 声明式组装所有组件
	// Container 自动解析依赖、拓扑排序、按序启动、逆序停止
	// NewMiddlewareComponent 注册后，ServerComponent.OnStart 自动收集并应用中间件链
	// （详见 components/middleware.go 注释）
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewRegistryComponent(memory.New()),
		components.NewMiddlewareComponent(recovery.New()),
		components.NewServerComponent(httpdriver.NewHTTP(httpdriver.Mux(mux))),
		components.NewServiceComponent(),
		components.WithStopTimeout(5*time.Second), // 可选：设置优雅关闭超时
	)

	app.Run()
}

// curl http://localhost:8080/hello
// curl http://localhost:8080/health
