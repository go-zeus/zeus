// api-1 API 入口服务（调用链起点；接收 gateway 转发的请求）
//
// 业务职责：模拟外部 API 入口（认证 / 限流 / 协议转换 等）。验证后调 srv-1。
// 调用链：gateway → api-1 → srv-1 → srv-2 → srv-3
//
// 启动参数（环境变量）：
//   - CLUSTER   ：集群名（default / user.v1.1 / order.v2 / batch.v3，默认 default）
//   - PORT      ：监听端口（默认 9000）
//   - GATEWAY_URL：gateway 地址（默认 http://localhost:8080）
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwdisc"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwreg"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/srvcfg"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/routing"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

func main() {
	cluster := srvcfg.Env("CLUSTER", routing.Default)
	port := srvcfg.EnvInt("PORT", 9000)
	gatewayURL := srvcfg.Env("GATEWAY_URL", "http://localhost:8080")
	instanceID := fmt.Sprintf("api1-%s-%s", cluster, srvcfg.Hostname())

	// 1. 创建调下游 srv-1 的 client
	srv1Client := client.NewClient("srv1",
		client.Discovery(gwdisc.New(gatewayURL)),
		client.LoadBalance(roundrobin.New()),
	)

	// 2. 业务 handler
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		c := routing.FromContext(r.Context())
		log.Info("api1[%s] /login cluster=%s", cluster, c)

		// 调下游 srv-1：client.Do 自动透传 X-Zeus-Cluster；
		// srv1 若无对应 cluster 实例，client 自动降级到 default
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://srv1/login", nil)
		resp, err := srv1Client.Do(req)
		if err != nil {
			http.Error(w, "srv1 call failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		srv1Body, _ := io.ReadAll(resp.Body)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"service":    "api1",
			"cluster":    c,
			"version":    version(cluster),
			"action":     "api_entry",
			"downstream": json.RawMessage(srv1Body),
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// 3. 启动 HTTP server
	srv := httpdriver.NewHTTP(
		httpdriver.Mux(mux),
		httpdriver.Port(port),
		httpdriver.IP(srvcfg.LocalIP()),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
			log.Error("api1 server error: %v", err)
		}
	}()

	// 4. 注册到 gateway
	regClient := gwreg.New(gatewayURL)
	regCtx, regCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer regCancel()
	if err := regClient.Register(regCtx, gwapi.Instance{
		ID:       instanceID,
		Name:     "api1",
		Cluster:  cluster,
		Protocol: "http",
		IP:       srvcfg.LocalIP(),
		Port:     port,
	}); err != nil {
		log.Fatal(fmt.Sprintf("api1 register failed: %v", err))
	}

	log.Info("api1[%s] ready on :%d", cluster, port)

	// 5. 等信号 → 反注册 → 优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("api1 received signal %v, shutting down...", sig)

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
	// user.v1.1 / order.v2 / batch.v3 等业务集群直接返回 cluster 名
	return cluster
}
