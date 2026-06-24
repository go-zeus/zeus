// Example: L3 类型装配（app.NewApp + WithXxx Option 模式）
//
// 与 L4（autoapp-multi/main.go）的对照：
//   - L4：components.NewApp(NewLogComponent(...), NewServerComponent(...), ...) —— 命令式逐个 NewXxxComponent
//   - L3：app.NewApp(AddServer(s), WithLogger(l), WithRegistry(r), ...) —— 扁平化 Option 模式
//
// 心智差异：
//   - L3 用户**不应感知** Component / Container / Lifecycle 等内部概念
//   - L3 是 L4 的"语法糖"，底层 100% 复用 L4（不绕过 Container）
//   - L3 用户在参数末尾可直接追加 components.NewXxxComponent(...) 实现渐进升级到 L4
//
// 演示场景：双 HTTP Server（业务 :9080 + 管理 :9081）+ 自定义 Logger + recovery middleware
//
// 启动后访问：
//
//	curl http://localhost:9080/ping              → "pong"
//	curl http://localhost:9080/panic             → 500（被 recovery 拦截）
//	curl http://localhost:9081/admin/health      → "admin ok"
//
// 优雅关闭：Ctrl+C 或 kill -INT <pid>
package main

import (
	"net/http"
	"time"

	"github.com/go-zeus/zeus/app"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/registry/memory"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

func main() {
	// —— 业务 Server（端口 9080）——
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	apiMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// 故意制造 panic 路径，验证 recovery 中间件生效
	apiMux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("intentional demo panic")
	})

	apiServer := httpdriver.NewHTTP(
		httpdriver.Mux(apiMux),
		httpdriver.Port(9080),
	)

	// —— 管理 Server（端口 9081）——
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("admin ok"))
	})

	adminServer := httpdriver.NewHTTP(
		httpdriver.Mux(adminMux),
		httpdriver.Port(9081),
		httpdriver.WithoutAutoClustering(), // 管理后台不需 cluster 路由
	)

	// —— L3 类型装配入口 ——
	// AddServer 可多次调用追加（每个 server 生成一个 Instance，共享 Name，区分 Protocol）
	// WithMiddleware 显式声明中间件链（L3 不自动包装 recovery/requestid/accesslog）
	a := app.NewApp(
		app.AddServer(apiServer),
		app.AddServer(adminServer),
		app.WithRegistry(memory.New()),
		app.WithMiddleware(recovery.New()),
		app.WithServiceName("typed-demo"),
		app.WithServiceCluster("default"),
		app.WithStopTimeout(15*time.Second),
	)

	// 返回 *components.App，能直接调用 .Run() / .RunWithContext(ctx) / .Container() / .Get(name)
	// L3 → L4 升级零成本：在参数末尾追加 components.NewXxxComponent(...) 即可
	a.Run()
}
