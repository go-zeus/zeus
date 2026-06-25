# Zeus Examples

22 个独立可运行的示例，按 4 层渐进暴露 API 组织：

| 示例 | 学习目标 |
|---|---|
| **L1 — 5 行启动** | |
| [01-hello](01-hello/) | 最小 HTTP 服务，体验零配置启动 |
| **L2 — 配置驱动** | |
| [02-with-registry](02-with-registry/) | URL scheme 切换注册中心（memory / etcd） |
| [04-config-driven](04-config-driven/) | URL scheme 切换 cache/database/mq |
| [10-config](10-config/) | 配置加载（file loader） |
| **L3 — 类型装配** | |
| [03-typed](03-typed/) | `app.NewApp` + `WithXxx` Option 模式 |
| [05-autoapp](05-autoapp/) | 自动装配最小示例 |
| [06-autoapp-full](06-autoapp-full/) | 自动装配完整示例 |
| [07-autoapp-multi](07-autoapp-multi/) | 多 Server 单 App（HTTP 双端口） |
| [08-client](08-client/) | HTTP 客户端 + 服务发现 + 集群路由 |
| [09-middleware](09-middleware/) | 中间件链组合 |
| [11-proxy](11-proxy/) | 多协议反向代理（HTTP/WS/SSE） |
| [12-cluster-routing](12-cluster-routing/) | X-Zeus-Cluster 端到端路由 |
| **L4 — 完全控制** | |
| [00-app-quickstart](00-app-quickstart/) | `components.NewApp` 手动装配 |
| **功能域示例** | |
| [13-database](13-database/) | database/sql + tx_id 透传 |
| [14-cache](14-cache/) | cache/memory + TTL |
| [15-mq](15-mq/) | mq/memory 进程内事件总线 |
| [16-job](16-job/) | job/interval 固定间隔任务 |
| [17-job-cron](17-job-cron/) | plugins/job/cron cron 表达式 |
| [18-propagation](18-propagation/) | W3C Baggage 全链路传播 |
| [19-observability](19-observability/) | metrics + trace + log 三件套 |
| [21-registry-etcd](21-registry-etcd/) | plugins/registry/etcd |
| **完整端到端** | |
| [20-full-demo](20-full-demo/) | gateway + 3 srv + frontend 端到端演示 |

## 使用方式

每个示例是独立 module，可直接复制使用：

```bash
cd examples/01-hello
go run .
```

具体启动参数与测试方式见每个目录下 `main.go` 顶部注释。

## Docker 一键启动

**有外部依赖的示例**（etcd / jaeger / prometheus / grafana）自带 `docker-compose.yml`，零配置启动：

| 示例 | 启动命令 | 访问 |
|---|---|---|
| [19-observability](19-observability/) | `docker compose -f examples/19-observability/docker-compose.yml up --build` | Grafana http://localhost:3000 (admin/admin)，Prometheus http://localhost:9090 |
| [21-registry-etcd](21-registry-etcd/) | `docker compose -f examples/21-registry-etcd/docker-compose.yml up --build` | http://localhost:18080 |
| [20-full-demo](20-full-demo/deploy/docker-compose.yml) | `docker compose -f examples/20-full-demo/deploy/docker-compose.yml up --build` | 前端 http://localhost:8088，gateway http://localhost:8080 |

**单进程示例**（01-hello / 03-typed 等）没有 compose，但可用通用 Docker 命令启动：

```bash
# 在任一单进程 example 目录下，用 Go 官方镜像直接 run
cd examples/01-hello
docker run --rm -p 8080:8080 -v "$PWD":/app -w /app golang:1.25 go run .

# 或本地编译后丢 scratch（最小镜像）
CGO_ENABLED=0 go build -o app .
cat > Dockerfile.scratch <<'EOF'
FROM gcr.io/distroless/static-debian12
COPY app /app
ENTRYPOINT ["/app"]
EOF
docker build -t my-zeus-app -f Dockerfile.scratch . && docker run --rm -p 8080:8080 my-zeus-app
```

按示例端口调整 `-p` 映射（如 03-typed 用 9001、08-client 用 18081，详见各 `main.go`）。

## 学习路径建议

1. **新手入门**：01-hello → 02-with-registry → 03-typed → 07-autoapp-multi
2. **进阶实战**：11-proxy → 12-cluster-routing → 19-observability
3. **高级用法**：20-full-demo（完整 K8s 部署）→ 18-propagation（baggage）
