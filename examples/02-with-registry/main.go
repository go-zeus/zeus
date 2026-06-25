// L2 示例：配置驱动 + 集群路由
//
// 通过 Config 字段定制服务名、集群、端口，演示灰度发布场景。
// 启动两个实例（不同 cluster）观察注册中心聚合。
//
// 启动：
//
//	# 终端 1：default 集群实例
//	CLUSTER=default PORT=9001 go run .
//
//	# 终端 2：canary 集群实例
//	CLUSTER=canary PORT=9002 go run .
//
// 测试：
//
//	curl http://localhost:9001/ping                 # default 链路
//	curl http://localhost:9002/ping                 # canary 链路
//	curl -H "X-Zeus-Cluster: canary" http://localhost:9001/ping   # 走 canary 实例
package main

import (
	"net/http"
	"os"
	"strconv"

	"github.com/go-zeus/zeus/app"
)

func main() {
	cfg := &app.Config{
		Name:    "demo-svc",
		Port:    envInt("PORT", 9001),
		Cluster: envStr("CLUSTER", "default"),
		// L2 不指定 Registry → 默认 memory（L3+ 才走 etcd）
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong from " + cfg.Cluster))
	})

	app.Run(cfg, mux)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
