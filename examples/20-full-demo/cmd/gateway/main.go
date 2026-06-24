// gateway 流量入口 + 嵌入式服务注册中心
//
// 三合一职责：
//  1. 注册中心：内嵌 zeus memory registry，暴露 HTTP /internal/register 供 srv 自注册
//  2. 服务发现 API：GET /api/services 返回所有实例 JSON（前端可视化用）
//  3. 反向代理：其他路径通过 zeus proxy 转发到 srv-1，按 X-Zeus-Cluster 路由
//
// 启动参数：
//   - PORT ：gateway 监听端口（默认 8080）
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/proxy"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/routing"
	"github.com/go-zeus/zeus/types"
)

// instanceCache 本地缓存 id → 完整 Instance，用于反注册时还原（memory registry 按 Name 删除）
type instanceCache struct {
	mu   sync.Mutex
	data map[string]*types.Instance
}

func newCache() *instanceCache { return &instanceCache{data: make(map[string]*types.Instance)} }

func (c *instanceCache) Set(ins *types.Instance) {
	c.mu.Lock()
	c.data[ins.ID] = ins
	c.mu.Unlock()
}

func (c *instanceCache) Get(id string) (*types.Instance, bool) {
	c.mu.Lock()
	ins, ok := c.data[id]
	c.mu.Unlock()
	return ins, ok
}

func main() {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if n := parseInt(p); n > 0 {
			port = n
		}
	}

	reg := memory.New()
	dis := reg.(registry.Discovery)
	cache := newCache()

	// 转发到 api-1 的反向代理（带集群路由）
	api1Selector := proxy.NewDiscoverySelector("api1", dis, roundrobin.New())
	reverseProxy := proxy.New(proxy.WithSelector(api1Selector))

	// 路由分发
	mux := http.NewServeMux()

	// === 注册中心 API（仅 srv 调用） ===
	mux.HandleFunc("/internal/register", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleRegister(reg, cache, w, r)
		case http.MethodDelete:
			handleDeregister(reg, cache, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// === 可视化 API（前端调用） ===
	mux.HandleFunc("/api/services", func(w http.ResponseWriter, r *http.Request) {
		handleServices(reg, w, r)
	})

	// === 其他路径反向代理到 api-1 ===
	mux.Handle("/", reverseProxy)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: corsMiddleware(mux),
	}

	go func() {
		log.Info("gateway listening on :%d", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(fmt.Sprintf("gateway server error: %v", err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Info("gateway received signal %v, shutting down...", sig)

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}

// handleRegister 处理 srv 自注册
func handleRegister(reg registry.Registrar, cache *instanceCache, w http.ResponseWriter, r *http.Request) {
	var req gwapi.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	ins := req.Instance
	if ins.Cluster == "" {
		ins.Cluster = routing.Default
	}
	t := &types.Instance{
		ID:       ins.ID,
		Name:     ins.Name,
		Cluster:  ins.Cluster,
		Protocol: ins.Protocol,
		IP:       ins.IP,
		Port:     ins.Port,
	}
	if err := reg.Register(r.Context(), t); err != nil {
		log.Error("gateway register failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cache.Set(t)
	log.Info("gateway: instance registered name=%s cluster=%s endpoint=%s:%d",
		ins.Name, ins.Cluster, ins.IP, ins.Port)
	json.NewEncoder(w).Encode(gwapi.RegisterResponse{OK: true})
}

// handleDeregister 处理 srv 反注册
func handleDeregister(reg registry.Registrar, cache *instanceCache, w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	ins, ok := cache.Get(id)
	if !ok {
		// 缓存未命中（重启场景）：返回成功，避免 srv 阻塞关闭
		log.Warn("gateway: deregister id=%s not in cache (skip)", id)
		json.NewEncoder(w).Encode(gwapi.RegisterResponse{OK: true})
		return
	}
	if err := reg.Deregister(r.Context(), ins); err != nil {
		log.Error("gateway deregister failed: %v", err)
	}
	log.Info("gateway: instance deregistered id=%s name=%s", id, ins.Name)
	json.NewEncoder(w).Encode(gwapi.RegisterResponse{OK: true})
}

// handleServices 返回所有服务的实例列表（按 name 分组）
func handleServices(reg registry.Registrar, w http.ResponseWriter, r *http.Request) {
	dis := reg.(registry.Discovery)
	// 硬编码已知服务名（demo 简化）；真实场景应让 registry 暴露 ListServices
	knownNames := []string{"api1", "srv1", "srv2", "srv3"}
	resp := gwapi.ServicesResponse{Services: make(map[string][]gwapi.Instance)}
	for _, name := range knownNames {
		entry, err := dis.GetService(r.Context(), name)
		if err != nil || entry == nil {
			continue
		}
		for _, ins := range entry.Instances {
			resp.Services[name] = append(resp.Services[name], gwapi.Instance{
				ID:       ins.ID,
				Name:     ins.Name,
				Cluster:  ins.Cluster,
				Protocol: ins.Protocol,
				IP:       ins.IP,
				Port:     ins.Port,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// corsMiddleware 给所有响应加 CORS 头（前端跨域用）
func corsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Zeus-Cluster")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
