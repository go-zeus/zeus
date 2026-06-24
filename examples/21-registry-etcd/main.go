// Example: registry-etcd
//
// 演示如何用 components 自动装配 + etcd 注册中心，让 ServiceComponent 自动把
// HTTP server 注册到 etcd；进程退出时自动反注册（lease 到期 + 显式 Deregister 双保险）。
//
// 启动前需准备 etcd：
//  1. 本机 etcd（localhost:2379）
//  2. 或远程 etcd，用环境变量指定：ZEUS_ETCD_ENDPOINT=127.0.0.1:2379
//  3. 或修改下方 WithEndpoints 自定义
//
// 启动：
//
//	go run .
//
// 验证（另开终端）：
//
//	# 查看 etcd 中已注册的实例（在 example 运行期间）
//	go run ./cmd/check
//
//	# 访问服务
//	curl http://localhost:18080/
//	curl http://localhost:18080/health
//
// 按 Ctrl+C 退出，etcd 中对应 key 应在 Deregister 完成后立即被删除；
// 异常退出（kill -9）时 lease 到期（默认 15s）后自动删除。
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-zeus/zeus/components"
	logslog "github.com/go-zeus/zeus/log/slog"
	zeushttp "github.com/go-zeus/zeus/server/http"

	etcd "github.com/go-zeus/zeus/plugins/registry/etcd"
)

func main() {
	// 1) etcd 端点：优先环境变量，其次默认 127.0.0.1:2379
	endpoint := os.Getenv("ZEUS_ETCD_ENDPOINT")
	if endpoint == "" {
		endpoint = etcd.DefaultEndpoint
	}
	log.Printf("using etcd endpoint: %s", endpoint)

	// 2) 构造 etcd registry
	//    - WithTTL 控制健康检查灵敏度（越小下线越快，但网络抖动易误判）
	//    - KeepAlive 自动续约，正常退出时 Deregister，异常退出 lease 到期自动反注册
	registry := etcd.New(
		etcd.WithEndpoints(endpoint),
		etcd.WithTTL(15*time.Second),
	)

	// 3) HTTP server 用 18080（避免与开发环境 8080 等冲突）
	srv := zeushttp.NewHTTP(zeushttp.Port(18080))

	// 4) 自动装配：Log → Registry → Server → Service（拓扑序）
	//    ServiceComponent 会从 Registry 拿 Registrar、从 Server 拿 Endpoint，
	//    自动生成 Instance 调用 Registrar.Register
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewRegistryComponent(registry),
		components.NewServerComponent(srv),
		components.NewServiceComponent(
			components.WithServiceName("zeus-etcd-demo"),
			components.WithServiceCluster("default"),
		),
		// 关闭超时：留足时间让 etcd Deregister 完成
		components.WithStopTimeout(10*time.Second),
	)

	// 5) Run 阻塞直到 SIGTERM/SIGINT/SIGQUIT
	//    退出时逆序：Service.OnStop(Deregister) → Server.OnStop(stop) → Registry.OnStop(Close)
	//    启动失败（如 etcd 不可达）会以非零码退出，便于编排系统重启
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}
