# Example: 19-observability
#
# 多阶段构建（otel 客户端要求 Go 1.23+，本 example go.mod 声明 1.25）：
#   docker build -t zeus-observability -f examples/19-observability/Dockerfile .
#   docker run --rm -p 18090:18090 --network=zeus-obs zeus-observability
#
# 与 docker-compose.yml 配合：
#   docker compose -f examples/19-observability/docker-compose.yml up --build

FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -trimpath -ldflags="-s -w" -o /out/server .

# === 运行阶段 ===
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

WORKDIR /app
COPY --from=builder /out/server /app/server

EXPOSE 18090

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://127.0.0.1:18090/metrics || exit 1

ENTRYPOINT ["/app/server"]
