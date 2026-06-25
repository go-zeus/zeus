// Example: observability
//
// 演示 metrics + trace + log 三件套联动：
//   - HTTP server 用 18090 端口
//   - 中间件链：recovery → tracing → metrics（顺序很重要）
//     recovery 最外层兜底 panic；tracing 创建 span；metrics 计数
//   - Prometheus /metrics 端点暴露计数
//   - OTel tracer 默认用 stdout exporter（输出到 stderr，便于本地验证）
//
// 中间件装配说明：
//   - 简单场景：用 components.NewMiddlewareComponent(recovery.New()) 自动应用
//     （ServerComponent.OnStart 收集所有 Interceptor 按字典序注入，无需手动包装）
//   - 严格顺序场景（如本示例）：用 middleware.NewChain + httpdriver.ChainHandler 显式控制
//     保证 recovery 最外层捕获所有 panic，tracing 第二层记录完整 span 生命周期
//
// 启动：
//
//	go run .
//
// 验证：
//
//	# 多次访问触发计数
//	curl http://localhost:18090/ping
//	curl http://localhost:18090/ping
//	curl -H "X-Zeus-Cluster: canary" http://localhost:18090/ping
//	curl http://localhost:18090/error   # 触发 500
//
//	# 抓取 Prometheus 指标
//	curl http://localhost:18090/metrics | grep zeus_requests_total
//
//	# OTel span 输出在 stderr（含 zeus.cluster attribute、ERROR status）
//
// Ctrl+C 退出。
package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/plugins/metrics/prometheus"
	metricsmw "github.com/go-zeus/zeus/plugins/middleware/metrics"
	tracingmw "github.com/go-zeus/zeus/plugins/middleware/tracing"
	"github.com/go-zeus/zeus/plugins/trace/otel"
	zeushttp "github.com/go-zeus/zeus/server/http"
)

func main() {
	// 1) 构造可观测三件套
	//    - meter：Prometheus，namespace=zeus
	//    - tracer：OTel，service=zeus-observability-demo，默认走 stdout exporter
	meter := prometheus.New(prometheus.WithNamespace("zeus"))
	tracer := otel.New(
		otel.WithServiceName("zeus-observability-demo"),
		otel.WithServiceVersion("v0.1.0"),
	)

	// 2) 构造业务 mux
	//    - /ping：固定 200
	//    - /error：固定 500（测试 record error + ERROR status）
	//    - /metrics：Prometheus 抓取端点
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "simulated error", http.StatusInternalServerError)
	})
	mux.Handle("/metrics", prometheus.HTTPHandler())

	// 3) 中间件链（顺序：recovery → tracing → metrics，外→内）
	//    严格顺序的场景应手动 ChainHandler：
	//    - recovery 最外层兜底 panic（防 handler 异常崩溃）
	//    - tracing 创建 span（包含完整请求生命周期，含 panic 时 span status=ERROR）
	//    - metrics 计数（带 cluster/method/status 维度，仅记录真正写出的响应）
	//
	//    若用 components.NewMiddlewareComponent 自动应用，顺序按字典序：
	//    metrics → recovery → tracing，tracing 会在最内层，panic 时 span 可能丢失。
	chain := middleware.NewChain(
		recovery.New(),
		tracingmw.New(tracer),
		metricsmw.New(meter),
	)

	srv := zeushttp.NewHTTP(
		zeushttp.Mux(zeushttp.ChainHandler(mux, chain)),
		zeushttp.Port(18090),
	)

	// 4) 自动装配：Log → Metrics → Trace → Server → Service
	//    Metrics/Trace 组件 OnStop 会调用 Close() 释放资源（如 OTel batch flush）
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewMetricsComponent(meter),
		components.NewTraceComponent(tracer),
		components.NewServerComponent(srv),
	)

	log.Info("observability demo starting at http://localhost:18090")
	log.Info("  curl http://localhost:18090/ping")
	log.Info("  curl -H \"X-Zeus-Cluster: canary\" http://localhost:18090/ping")
	log.Info("  curl http://localhost:18090/metrics | grep zeus_requests_total")
	log.Info("  Press Ctrl+C to stop (OTel spans flushed on shutdown)")

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}
