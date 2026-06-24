// Example: 多 Server 单 App 演示（HTTP 双端口 + 注册两条 Instance）
//
// 演示场景：同一进程内同时启动两个 HTTP server（监听不同端口），
// 框架为每个 server 生成一个 Instance 并注册到注册中心（带 protocol 字段）。
//
// 真实场景：把第二个 server 替换为 grpc.NewGRPC(grpc.Port(9002)) 即可
// 实现 HTTP + gRPC 同进程多协议注册（无需改其他代码）。
//
// 启动后访问：
//
//	curl http://localhost:9001/hello  → "hello from api"
//	curl http://localhost:9002/admin  → "admin endpoint"
package main

import (
	"net/http"

	"github.com/go-zeus/zeus/components"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/registry/memory"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

func main() {
	// 第一个 HTTP server：业务 API（端口 9001）
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello from api"))
	})
	apiServer := httpdriver.NewHTTP(
		httpdriver.Mux(apiMux),
		httpdriver.Port(9001),
	)

	// 第二个 HTTP server：管理后台（端口 9002）
	// 真实场景这里通常是 grpc.NewGRPC(grpc.Port(9002))
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("admin endpoint"))
	})
	adminServer := httpdriver.NewHTTP(
		httpdriver.Mux(adminMux),
		httpdriver.Port(9002),
		httpdriver.WithoutAutoClustering(), // 管理后台不需 cluster 路由
	)

	// 声明式组装：传入两个 server，框架会注册两条 Instance（共享 name）
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewRegistryComponent(memory.New()),
		components.NewServerComponent(apiServer, adminServer),
		components.NewServiceComponent(
			components.WithServiceName("multi-app"),
		),
	)
	app.Run()

	// 启动后，注册中心会包含两条 Instance：
	//   {name=multi-app, protocol=http, port=9001}
	//   {name=multi-app, protocol=http, port=9002}
}
