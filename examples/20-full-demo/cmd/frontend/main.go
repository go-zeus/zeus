// 简易静态文件服务器（替代 nginx，避免依赖基础镜像）
//
// 业务逻辑：
//  1. 托管 /app/frontend/ 下的静态文件（index.html, app.js, style.css）
//  2. /api/* /login 反向代理到 gateway（与原 nginx 配置一致）
//
// 用法：CLUSTER=frontend PORT=80 GATEWAY_URL=http://gateway:8080 ./frontend
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const defaultStaticDir = "/app/frontend"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = defaultStaticDir
	}
	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://gateway:8080"
	}

	mux := http.NewServeMux()

	// 反向代理 /api/ 和 /login 到 gateway
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequestWithContext(r.Context(), r.Method, gatewayURL+r.URL.Path, r.Body)
		req.Header = r.Header.Clone()
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		// 透传响应头（CORS）
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Zeus-Cluster")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
	mux.HandleFunc("/api/", proxy)
	mux.HandleFunc("/login", proxy)

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// 静态文件
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// 防止路径穿越
		path := filepath.Clean(r.URL.Path)
		if strings.HasPrefix(path, "..") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		full := filepath.Join(staticDir, path)
		// 目录默认 index.html
		if fi, err := os.Stat(full); err == nil && fi.IsDir() {
			full = filepath.Join(full, "index.html")
		}
		http.ServeFile(w, r, full)
	}))

	log.Printf("frontend serving %s on :%s (gateway=%s)", staticDir, port, gatewayURL)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
