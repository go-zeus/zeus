# 宿主机预编译模式镜像（离线/内网环境专用）
#
# 与 service.Dockerfile（多阶段 golang 构建）的区别：
#   - 二进制在宿主机交叉编译（GOOS=linux GOARCH=amd64 CGO_ENABLED=0），容器内仅 COPY
#   - 运行时基础镜像可参数化（内网 registry 的 busybox），不依赖 Docker Hub 的 golang/alpine
#   - 适用于无法访问 Docker Hub 的环境（如公司内网，Makefile BASE_REGISTRY 模式自动使用本文件）
#
# Build context 必须是 zeus repo 根目录：
#   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o examples/20-full-demo/bin/srv1 ./examples/20-full-demo/cmd/srv1
#   docker build -t zeus-srv1 \
#     -f examples/20-full-demo/docker/service-hostbuild.Dockerfile \
#     --build-arg SVC=srv1 \
#     --build-arg RUNTIME_IMAGE=busybox:latest ..

# 运行时基础镜像（Makefile 会传内网地址，如 hub.wang.dd:5000/library/busybox:latest）
# busybox 自带 wget（健康检查用）；zeus 服务二进制 CGO_ENABLED=0 纯静态，busybox 可直接运行
ARG RUNTIME_IMAGE=busybox:latest
FROM ${RUNTIME_IMAGE}

WORKDIR /app

ARG SVC=srv1
COPY examples/20-full-demo/bin/${SVC} /app/server

# 健康检查（每个服务都暴露 /health；PORT 由部署环境注入）
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/server"]
