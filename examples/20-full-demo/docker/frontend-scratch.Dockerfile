# 前端 nginx 镜像替代方案
#
# 由于环境拉不到 nginx:alpine，前端用 caddy 或简单 Python http.server 替代。
# 这里用 busybox httpd 作为静态文件服务器（busybox 二进制 + 静态文件，scratch 镜像）
#
# 实际部署时简化：直接用本地编译的 Go HTTP server 托管前端文件
# 详细见 cmd/frontend/main.go

FROM scratch

# 前端静态文件（app.js/index.html/style.css）
COPY frontend /app/frontend

# Go 静态文件服务器二进制（cmd/frontend）
COPY frontend-server /app/server

# CA 证书（HTTPS 反代 gateway 时使用）
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 80
ENTRYPOINT ["/app/server"]
