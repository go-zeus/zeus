// Example: propagation
//
// 演示 W3C Baggage 兼容的用户自定义 K-V 全链路传播。
// 用户在源头注入 tenant.id / feature.flag / region 等 K-V，框架自动透传：
//   - server/http 入口自动 extract Baggage header → ctx
//   - client 出口自动 inject ctx baggage → Baggage header
//   - log 自动从 ctx 读取 baggage entries 写成 Field
//   - tracing 自动写 span attribute
//
// 启动：
//
//	go run .
//
// 验证：
//
//	# 带 baggage header 请求（模拟上游传播）
//	curl -H "Baggage: tenant.id=acme,feature.flag=beta" http://localhost:18091/api
//
//	# 服务端日志会自动带 tenant.id=acme feature.flag=beta 字段
//	# 下游 client 调用时，Baggage header 自动透传给后端
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
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/routing"
	zeushttp "github.com/go-zeus/zeus/server/http"
)

func main() {
	mux := http.NewServeMux()

	// /api：从 ctx 读取 baggage entries，演示 log 自动带 K-V
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1) 自动读取 cluster（routing 仍是显式 API，但底层同步到 baggage）
		cluster := routing.FromContext(ctx)

		// 2) 读取用户自定义 baggage entries
		tenant, _ := propagation.Get(ctx, "tenant.id")
		feature, _ := propagation.Get(ctx, "feature.flag")

		// 3) log 自动带 cluster / tenant.id / feature.flag 字段（用户无需手动拼）
		log.Default().Log(ctx, log.LevelInfo, "received request")

		// 4) 业务逻辑：根据 baggage 内容路由不同分支
		if feature == "beta" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "beta branch: cluster=%s tenant=%s\n", cluster, tenant)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "stable branch: cluster=%s tenant=%s\n", cluster, tenant)
	})

	// /inject：演示业务代码主动注入 baggage
	// 用户调 propagation.With(ctx, "k", "v") 后，下游服务自动看到
	mux.HandleFunc("/inject", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// 模拟业务层注入新 K-V（如根据 user token 决定 user.tier）
		ctx = propagation.With(ctx, "user.tier", "premium")
		// 后续调用 client.Do(ctx, req) 时会自动透传 user.tier 给下游
		// 这里直接 log 验证字段自动出现
		log.Default().Log(ctx, log.LevelInfo, "after inject user.tier")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "user.tier injected; check log for 'user.tier=premium'")
	})

	chain := middleware.NewChain(recovery.New())
	srv := zeushttp.NewHTTP(
		zeushttp.Mux(zeushttp.ChainHandler(mux, chain)),
		zeushttp.Port(18091),
	)

	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewServerComponent(srv),
	)

	log.Info("propagation demo starting at http://localhost:18091")
	log.Info("  curl -H 'Baggage: tenant.id=acme,feature.flag=beta' http://localhost:18091/api")
	log.Info("  curl -H 'Baggage: tenant.id=globex' http://localhost:18091/api")
	log.Info("  curl http://localhost:18091/inject")
	log.Info("  Press Ctrl+C to stop")

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}
