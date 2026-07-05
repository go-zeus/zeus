# 19-observability · metrics + trace + log 三件套联动

演示 Zeus 可观测体系：构造期注入 tracer/meter → 中间件链自动埋点 → Prometheus 抓取 + OTel span 输出。本示例带 `docker-compose.yml`（Jaeger + Prometheus + Grafana）。

## 演示场景

- HTTP server `:18090`，中间件链 `recovery → tracing → metrics`（顺序重要）
- Prometheus `/metrics` 端点暴露计数
- OTel tracer 默认 stdout exporter（输出到 stderr，本地验证）
- `X-Zeus-Cluster` 自动写入 span attribute 与 metrics label

## 启动与测试

本地（stdout 验证）：

```bash
cd examples/19-observability
go run .
```

```bash
curl http://localhost:18090/ping                                   # 触发计数
curl -H "X-Zeus-Cluster: canary" http://localhost:18090/ping       # 带 cluster
curl http://localhost:18090/error                                  # 触发 500

curl http://localhost:18090/metrics | grep zeus_requests_total     # Prometheus 指标
# OTel span 输出在 stderr（含 zeus.cluster attribute、ERROR status）
```

完整栈（Jaeger + Prometheus + Grafana）：

```bash
docker compose -f examples/19-observability/docker-compose.yml up --build
# Grafana http://localhost:3000 (admin/admin)，Prometheus http://localhost:9090
```

## 核心代码

```go
meter := prometheus.New(prometheus.WithNamespace("zeus"))
tracer := otel.New(otel.WithServiceName("obs-demo"))

chain := middleware.NewChain(recovery.New(), tracingmw.New(tracer), metricsmw.New(meter))
srv := httpdriver.NewHTTP(httpdriver.Mux(httpdriver.ChainHandler(handler, chain)))

// /metrics 端点
mux.Handle("/metrics", prometheus.HTTPHandler())
```

优雅关闭时 `TraceComponent.OnStop` 调 `tracer.Close()`，OTel batch flush 完成。

## 衔接

- 中间件链独立用法 → [09-middleware](../09-middleware/)
- 完整集群部署（含可观测） → [20-full-demo](../20-full-demo/)
