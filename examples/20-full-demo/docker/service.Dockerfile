# 通用服务镜像（api1 / srv1 / srv2 / srv3 / gateway 共用）
#
# Build context 必须是 zeus repo 根目录（因为 full-demo 用 replace ../../ 引用主项目）：
#   docker build -t zeus-srv1 \
#     -f examples/20-full-demo/docker/service.Dockerfile \
#     --build-arg SVC=srv1 ..
#
# 多阶段构建：golang:alpine 编译 → alpine 运行（最终镜像 < 20MB）

FROM golang:1.22-alpine AS builder

# 国内构建可加 GOPROXY 加速
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0

WORKDIR /src
COPY . .

ARG SVC=srv1
RUN cd examples/20-full-demo/cmd/${SVC} && \
    go build -trimpath -ldflags="-s -w" -o /out/server .

# === 运行阶段 ===
FROM alpine:3.18

RUN apk add --no-cache ca-certificates wget

WORKDIR /app
COPY --from=builder /out/server /app/server

# 健康检查（每个服务都暴露 /health）
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/server"]
