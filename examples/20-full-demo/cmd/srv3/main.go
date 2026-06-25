// srv-3 支付服务（调用链终点）
//
// 业务职责：模拟订单支付。不调用下游，是 srv-1 → srv-2 → srv-3 的终点。
//
// 启动参数（环境变量）：
//   - CLUSTER   ：default / canary（默认 default）
//   - PORT      ：监听端口（默认 9003）
//   - GATEWAY_URL：gateway 地址（默认 http://gateway:8080）
//
// 注册流程：
//  1. 启动 HTTP server（zeus server/http，自动注入 X-Zeus-Cluster 到 ctx）
//  2. HTTP POST 到 gateway /internal/register 注册实例
//  3. SIGTERM 时 HTTP DELETE 反注册，然后优雅关闭 server
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwreg"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/srvcfg"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/routing"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

func main() {
	cluster := srvcfg.Env("CLUSTER", routing.Default)
	port := srvcfg.EnvInt("PORT", 9003)
	gatewayURL := srvcfg.Env("GATEWAY_URL", "http://localhost:8080")
	instanceID := fmt.Sprintf("srv3-%s-%s", cluster, srvcfg.Hostname())

	// 1. 业务 handler
	mux := http.NewServeMux()
	mux.HandleFunc("/pay", func(w http.ResponseWriter, r *http.Request) {
		c := routing.FromContext(r.Context())
		log.Info("srv3[%s] /pay cluster=%s", cluster, c)
		// 终点服务：直接返回支付结果
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"service": "srv3",
			"cluster": c, // 实际命中的 cluster（透传自流量标记）
			"version": version(cluster),
			"action":  "payment_processed",
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// 2. 启动 HTTP server（zeus server/http，默认自动注入 cluster）
	srv := httpdriver.NewHTTP(
		httpdriver.Mux(mux),
		httpdriver.Port(port),
		httpdriver.IP(srvcfg.LocalIP()),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
			log.Error("srv3 server error: %v", err)
		}
	}()

	// 3. 注册到 gateway（带重试，gateway 可能尚未就绪）
	regClient := gwreg.New(gatewayURL)
	regCtx, regCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer regCancel()
	if err := regClient.Register(regCtx, gwapi.Instance{
		ID:       instanceID,
		Name:     "srv3",
		Cluster:  cluster,
		Protocol: "http",
		IP:       srvcfg.LocalIP(),
		Port:     port,
	}); err != nil {
		log.Fatal(fmt.Sprintf("srv3 register failed: %v", err))
	}

	log.Info("srv3[%s] ready on :%d", cluster, port)

	// 4. 等信号 → 反注册 → 优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("srv3 received signal %v, shutting down...", sig)

	deregCtx, deregCancel := context.WithTimeout(context.Background(), 3*time.Second)
	regClient.Deregister(deregCtx, instanceID)
	deregCancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	srv.Stop(shutCtx)
}

// version 不同 cluster 返回不同版本标识（前端可视化区分用）
func version(cluster string) string {
	if cluster == routing.Default {
		return "v1-stable"
	}
	// 业务集群（batch.v3 等）直接返回 cluster 名
	return cluster
}
