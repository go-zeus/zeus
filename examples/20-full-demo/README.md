# Zeus Full Demo · 完整微服务集群路由示例

依据 `assets/traffic-flow.png` 流量图构建的端到端示例：**Gateway → srv-1（认证）→ srv-2（订单）→ srv-3（支付）**，每层都有 `default` 与 `canary` 双集群，按 `X-Zeus-Cluster` Header 端到端路由。

```
┌──────────────────────────────────────────────────────────────────┐
│                          [Gateway :8080]                          │
│   注册中心 + 反向代理 + 可视化 API                                 │
└────────────────┬─────────────────────────────────────────────────┘
                 │ 按 X-Zeus-Cluster 路由
        ┌────────┴────────┐
        ▼                 ▼
   [default]          [canary]
   ┌────────┐         ┌────────┐
   │ srv-1  │         │ srv-1  │   用户认证
   └───┬────┘         └───┬────┘
       │                  │
   ┌───▼────┐         ┌───▼────┐
   │ srv-2  │         │ srv-2  │   订单服务
   └───┬────┘         └───┬────┘
       │                  │
   ┌───▼────┐         ┌───▼────┐
   │ srv-3  │         │ srv-3  │   支付服务（终点）
   └────────┘         └────────┘
```

## 核心概念演示

| 概念 | 体现位置 |
|---|---|
| **服务注册/反注册** | srv 启动调 `POST /internal/register`，关闭调 `DELETE` |
| **Instance 数据模型** | Id/Name/Cluster/Protocol/Ip/Port/Metadata |
| **多协议多实例** | 每个 srv 一个 Instance，Protocol=http |
| **Cluster 路由** | 客户端 Header `X-Zeus-Cluster` → 实例 `Cluster` 字段匹配 |
| **Cluster 透传** | zeus server/http 入口自动注入 ctx → client 自动透传 Header |
| **优雅关闭** | SIGTERM → 反注册 → server.Shutdown（5s 超时） |

## 目录结构

```
examples/20-full-demo/
├── cmd/                            # 4 个可执行服务
│   ├── gateway/main.go             # 网关 + 嵌入式注册中心
│   ├── srv1/main.go                # 用户认证（调 srv2）
│   ├── srv2/main.go                # 订单（调 srv3）
│   └── srv3/main.go                # 支付（终点）
├── internal/
│   ├── gwapi/types.go              # 共享 JSON 类型
│   ├── gwreg/client.go             # HTTP 自注册客户端
│   ├── gwdisc/discovery.go         # HTTP discovery 适配 zeus registry.Discovery
│   └── srvcfg/env.go               # 环境变量工具
├── frontend/                       # 纯 HTML+JS 流量可视化
│   ├── index.html
│   ├── style.css
│   └── app.js
├── docker/
│   ├── service.Dockerfile          # 通用服务镜像（SVC=srv1/srv2/srv3/gateway）
│   └── frontend.Dockerfile         # nginx + 反向代理
├── deploy/
│   ├── docker-compose.yml          # 8 个服务编排
│   └── k8s/                        # 每个 srv 双 Deployment
│       ├── namespace.yaml
│       ├── gateway.yaml
│       ├── srv1.yaml
│       ├── srv2.yaml
│       ├── srv3.yaml
│       └── frontend.yaml
├── go.mod
├── Makefile
└── README.md（本文件）
```

## 快速开始

### 方式 1：Docker Compose（推荐，零依赖本地运行）

```bash
cd examples/20-full-demo
make up                # 构建 + 启动 8 个容器

# 访问
open http://localhost:8088   # 前端可视化
curl http://localhost:8080/api/services | jq   # 实例列表

# 测试路由
make test

# 停止
make down
```

### 方式 2：K8s（minikube / kind）

```bash
cd examples/20-full-demo
make k8s-images       # 构建并 load 5 个镜像
make k8s-apply        # 应用 manifests

# 获取前端访问地址
minikube service frontend -n zeus-demo --url
# 或 kind：kubectl port-forward -n zeus-demo svc/frontend 8088:80

# 卸载
make k8s-delete
```

### 方式 3：本地直接运行（无容器）

```bash
cd examples/20-full-demo
make build      # 编译 4 个二进制
make run        # 后台启动 1 gateway + 6 srv（default+canary 各 3 个）

# 前端单独跑
cd frontend && python3 -m http.server 8088

# 停止
make stop
```

## 端到端调用链验证

### default 链路（默认）

```bash
$ curl http://localhost:8080/login
{
  "service": "srv1",
  "cluster": "default",
  "version": "v1-stable",
  "action": "user_authenticated",
  "downstream": {
    "service": "srv2",
    "cluster": "default",
    "version": "v1-stable",
    "action": "order_created",
    "downstream": {
      "service": "srv3",
      "cluster": "default",
      "version": "v1-stable",
      "action": "payment_processed"
    }
  }
}
```

### canary 链路（Header 路由）

```bash
$ curl -H 'X-Zeus-Cluster: canary' http://localhost:8080/login
{
  "service": "srv1",
  "cluster": "canary",
  "version": "v2-canary",
  ...
  "downstream": {
    "service": "srv2",
    "cluster": "canary",
    "version": "v2-canary",
    ...
  }
}
```

注意：所有 3 层都命中 `canary`，证明 `X-Zeus-Cluster` 被端到端透传。

## 前端可视化说明

打开 `http://localhost:8088`：
- **拓扑图**：实时显示 gateway + srv1/2/3 × default/canary，节点显示实例数
- **按钮区**：发 default / canary 流量，发请求时高亮对应路径
- **调用链**：嵌套 JSON 格式化展示，直观看到 3 层 cluster 一致性

## 设计要点（KISS / DRY / SOLID 体现）

| 设计 | 体现 |
|---|---|
| **单一 Dockerfile** | `service.Dockerfile` 通过 `--build-arg SVC=...` 编译不同服务（DRY） |
| **服务自注册** | srv 通过 HTTP 调 gateway 注册，无外部依赖（不需要 etcd） |
| **HTTP Discovery 适配器** | `gwdisc.New(url)` 实现 `registry.Discovery`，无缝接入 zeus client |
| **cluster 自动透传** | server/http 入口注入 ctx → client 自动透传 Header，业务代码 0 改动 |
| **优雅关闭** | 信号 → 反注册 → server.Shutdown，5s 超时兜底 |
| **CORS 隔离** | gateway 加 CORS 头，前端可独立部署 |

## 与 zeus 组件库的对应

| 示例组件 | zeus 包 |
|---|---|
| HTTP server | `server/http`（`httpdriver.NewHTTP`） |
| HTTP client | `client.NewClient`（自动透传 cluster） |
| 反向代理 | `proxy.New` + `proxy.NewDiscoverySelector` |
| 内存注册中心 | `registry/memory` |
| 集群路由 | `routing` 包（HeaderCluster 常量 + ctx 注入） |
| 负载均衡 | `balancer/round_robin` |

## 已知限制（与生产差距）

1. **HTTP Discovery 性能**：每次调用拉取一次 `/api/services`，生产应换 etcd（带 watch）
2. **单 gateway**：注册中心是单点，生产应做 HA（etcd 集群）
3. **无 TLS**：示例简化，生产应在 gateway 终结 TLS
4. **无限流/熔断**：未集成 `ratelimit`/`circuitbreaker`，可参考 zeus 各 cluster 治理模块
5. **无追踪**：未集成 `trace`，可加 `plugins/middleware/tracing`

## 扩展练习

1. 把 `srv3` 改为 gRPC，用 `plugins/server/grpc`（保留 HTTP srv1/srv2，实现多协议混合）
2. 集成 `ratelimit/cluster` 给 canary 集群限流
3. 接入 etcd 替换嵌入式 memory registry（`plugins/registry/etcd`）
4. 把 frontend 的拓扑图升级为 D3.js 力导向图

---

**相关文档**：根目录 [CLAUDE.md](../../CLAUDE.md) 有 zeus 完整架构说明。
