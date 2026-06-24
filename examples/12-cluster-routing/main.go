// Example: 集群路由端到端示例（X-Zeus-Cluster）
//
// 本示例演示并行开发场景下，多项目共享同一套服务时的流量隔离：
//   - api-1[default] → srv-1[canary] → srv-2[canary] → srv-3[default]
//
// 单进程启动所有服务，模拟完整调用链：
//  1. srv2：注册两个 cluster（default + canary），分别监听不同端口返回不同标识
//  2. srv1：HTTP server（默认注入 cluster），handler 内用 client.NewClient 调 srv2
//  3. gateway：proxy 反向代理（基于服务发现 + 集群路由）
//
// 测试命令：
//
//	curl http://localhost:8081/ping                                  → default 链路
//	curl -H "X-Zeus-Cluster: canary" http://localhost:8081/ping      → canary 链路
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/proxy"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

func main() {
	ctx := context.Background()
	reg := memory.New()
	dis := reg.(registry.Discovery)

	// === srv2：注册 default 和 canary 两个 cluster，监听不同端口 ===
	// default cluster
	reg.Register(ctx, &types.Instance{
		ID: "srv2-default", Name: "srv2", Cluster: routing.Default, IP: "127.0.0.1", Port: 9101,
	})
	reg.Register(ctx, &types.Instance{
		ID: "srv2-canary", Name: "srv2", Cluster: "canary", IP: "127.0.0.1", Port: 9102,
	})

	srv2DefaultMux := http.NewServeMux()
	srv2DefaultMux.HandleFunc("/who", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "srv2[default]")
	})
	go http.ListenAndServe("127.0.0.1:9101", srv2DefaultMux)

	srv2CanaryMux := http.NewServeMux()
	srv2CanaryMux.HandleFunc("/who", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "srv2[canary]")
	})
	go http.ListenAndServe("127.0.0.1:9102", srv2CanaryMux)

	// === srv1：HTTP server，默认注入 cluster，handler 内通过 client 调 srv2 ===
	reg.Register(ctx, &types.Instance{
		ID: "srv1-default", Name: "srv1", Cluster: routing.Default, IP: "127.0.0.1", Port: 9001,
	})

	srv2Client := client.NewClient("srv2",
		client.Discovery(dis),
		client.LoadBalance(roundrobin.New()),
	)

	srv1Mux := http.NewServeMux()
	srv1Mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		// 当前请求的 cluster 已被 server/http 自动注入 ctx
		c := routing.FromContext(r.Context())
		log.Info("srv1 received ping, cluster=%s", c)

		// 用 client 调下游 srv2：client.Do 会从 ctx 读 cluster 路由到对应 cluster
		req, _ := http.NewRequestWithContext(r.Context(), "GET", "http://srv2/who", nil)
		resp, err := srv2Client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		fmt.Fprintf(w, "srv1[%s] -> %s", c, string(body[:n]))
	})
	go http.ListenAndServe("127.0.0.1:9001", srv1Mux)

	// === gateway：proxy 反向代理（基于服务发现 + 集群路由） ===
	gatewaySelector := proxy.NewDiscoverySelector("srv1", dis, roundrobin.New())
	p := proxy.New(proxy.WithSelector(gatewaySelector))

	log.Info("gateway listening on :8081")
	log.Info("test: curl -H 'X-Zeus-Cluster: canary' http://localhost:8081/ping")
	time.Sleep(200 * time.Millisecond) // 等待上游 server 启动
	log.Fatal(http.ListenAndServe(":8081", p))
}
