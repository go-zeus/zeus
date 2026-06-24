# Example: 21-registry-etcd
#
# 多阶段构建（Go 1.25 是 etcd v3.6 的硬性要求）：
#   docker build -t zeus-registry-etcd -f examples/21-registry-etcd/Dockerfile .
#   docker run --rm -p 18080:18080 --network=zeus-etcd zeus-registry-etcd
#
# 与 docker-compose.yml 配合：docker compose -f examples/21-registry-etcd/docker-compose.yml up

FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src

# 先拷依赖（利用层缓存）
COPY go.mod go.sum ./
RUN go mod download

# 再拷源码
COPY . .

RUN go build -trimpath -ldflags="-s -w" -o /out/server .

# === 运行阶段 ===
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

WORKDIR /app
COPY --from=builder /out/server /app/server

# 端口与 main.go 中的 18080 对齐
EXPOSE 18080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://127.0.0.1:18080/health || exit 1

ENTRYPOINT ["/app/server"]
