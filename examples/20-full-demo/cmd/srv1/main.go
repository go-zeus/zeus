// srv-1 用户认证服务（调用链入口；接收 gateway 转发的请求）
//
// 业务职责：模拟用户登录/鉴权。验证后调 srv-2 创建订单。
// srv-1 是调用链的起点：gateway → srv-1 → srv-2 → srv-3
//
// 启动参数（环境变量）：
//   - CLUSTER   ：default / canary（默认 default）
//   - PORT      ：监听端口（默认 9001）
//   - GATEWAY_URL：gateway 地址（默认 http://gateway:8080）
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
	port := srvcfg.EnvInt("PORT", 9001)
	gatewayURL := srvcfg.Env("GATEWAY_URL", "http://localhost:8080")
	instanceID := fmt.Sprintf("srv1-%s-%s", cluster, srvcfg.Hostname())

	// 1. 创建调下游 srv-2 的 client
	srv2Client := client.NewClient("srv2",
		client.Discovery(gwdisc.New(gatewayURL)),
		client.LoadBalance(roundrobin.New()),
	)

	// 2. 业务 handler
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		c := routing.FromContext(r.Context())
		log.Info("srv1[%s] /login cluster=%s", cluster, c)

		// 调下游 srv-2：client.Do 自动透传 X-Zeus-Cluster；
		// srv2 若无对应 cluster 实例，client 自动降级到 default
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://srv2/order", nil)
		resp, err := srv2Client.Do(req)
		if err != nil {
			http.Error(w, "srv2 call failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		srv2Body, _ := io.ReadAll(resp.Body)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"service":    "srv1",
			"cluster":    c,
			"version":    version(cluster),
			"action":     "user_authenticated",
			"downstream": json.RawMessage(srv2Body),
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
			log.Error("srv1 server error: %v", err)
		}
	}()

	// 4. 注册到 gateway
	regClient := gwreg.New(gatewayURL)
	regCtx, regCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer regCancel()
	if err := regClient.Register(regCtx, gwapi.Instance{
		ID:       instanceID,
		Name:     "srv1",
		Cluster:  cluster,
		Protocol: "http",
		IP:       srvcfg.LocalIP(),
		Port:     port,
	}); err != nil {
		log.Fatal(fmt.Sprintf("srv1 register failed: %v", err))
	}

	log.Info("srv1[%s] ready on :%d", cluster, port)

	// 5. 等信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("srv1 received signal %v, shutting down...", sig)

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
	// 业务集群（user.v1.1 / batch.v3 等）直接返回 cluster 名
	return cluster
}
