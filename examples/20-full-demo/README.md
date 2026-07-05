# Zeus Full Demo · 完整微服务集群路由示例

端到端示例：**Gateway → api1 → srv1 → srv2 → srv3** 四层业务链路，按 `X-Zeus-Cluster` Header 端到端路由，演示"有标识走标识、无标识走 default"的降级语义。

部署采用 **4 集群矩阵**（不是简单的 default/canary 双集群），每个业务服务部署到不同子集，覆盖典型灰度场景：

| 集群 | 含义 | api1 | srv1 | srv2 | srv3 |
|---|---|:---:|:---:|:---:|:---:|
| `default` | 稳定基线 | ✓ | ✓ | ✓ | ✓ |
| `user.v1.1` | 用户域灰度 | — | ✓ | ✓ | — |
| `order.v2` | 订单域灰度 | — | — | ✓ | — |
| `batch.v3` | 批处理灰度 | ✓ | ✓ | — | ✓ |

```
                         [Gateway :8080]
                   注册中心 + 反向代理 + 可视化 API
                              │
                              ▼  按 X-Zeus-Cluster 路由
                         [api1]  API 入口
                     default  |  batch.v3
                              │
                              ▼
                         [srv1]  用户认证
              default  |  user.v1.1  |  batch.v3
                              │
                              ▼
                         [srv2]  订单
              default  |  user.v1.1  |  order.v2
                              │
                              ▼
                         [srv3]  支付（终点）
                      default  |  batch.v3
```

请求带的 `X-Zeus-Cluster` 在每层"有则命中、无则降级 default"。例：`user.v1.1` 流量在 srv1/srv2 命中灰度实例，在 api1/srv3（未部署该集群）降级到 default。

## 端到端路由矩阵

发请求时指定 Header，观察每层实际命中的 cluster（client 自动降级用 `→` 标注）：

| `X-Zeus-Cluster` | api1 | srv1 | srv2 | srv3 |
|---|---|---|---|---|
| *(无)* | default | default | default | default |
| `user.v1.1` | default | **user.v1.1** | **user.v1.1** | default |
| `order.v2` | default | default | **order.v2** | default |
| `batch.v3` | **batch.v3** | **batch.v3** | default | **batch.v3** |

## 核心概念演示

| 概念 | 体现位置 |
|---|---|
| **服务注册/反注册** | 各服务启动调 `POST /internal/register`，关闭调 `DELETE` |
| **Instance 数据模型** | Id/Name/Cluster/Protocol/Ip/Port/Metadata |
| **多协议多实例** | 每个服务一个 Instance，Protocol=http；同名 Instance 按 Cluster 聚合 |
| **Cluster 路由** | client 读 ctx cluster → 选 cluster 实例；`X-Zeus-Cluster` Header 端到端透传 |
| **Cluster 降级** | 某层未部署请求 cluster 时，client 自动降级到 default（不报错） |
| **优雅关闭** | SIGTERM → 反注册 → server.Stop（5s 超时） |

## 目录结构

```
examples/20-full-demo/
├── cmd/                            # 6 个可执行服务
│   ├── gateway/main.go             # 网关 + 嵌入式注册中心 + 反向代理
│   ├── api1/main.go                # API 入口（调用链起点，调 srv1）
│   ├── srv1/main.go                # 用户认证（调 srv2）
│   ├── srv2/main.go                # 订单（调 srv3）
│   ├── srv3/main.go                # 支付（调用链终点）
│   └── frontend/main.go            # 静态文件托管 + /api/* 反代 gateway
├── internal/
│   ├── gwapi/types.go              # 共享 JSON 类型（Instance 等）
│   ├── gwreg/client.go             # HTTP 自注册客户端
│   ├── gwdisc/discovery.go         # HTTP discovery 适配 zeus registry.Discovery
│   └── srvcfg/env.go               # 环境变量工具
├── frontend/                       # 前端静态资源（由 cmd/frontend 托管）
│   ├── index.html
│   ├── style.css
│   └── app.js
├── docker/
│   ├── service.Dockerfile          # 通用服务镜像（SVC=api1/srv1/srv2/srv3/gateway）
│   └── frontend.Dockerfile         # frontend 镜像（Go 二进制，替代 nginx）
├── deploy/
│   ├── docker-compose.yml          # 12 个服务编排（gateway + 11 业务/frontend）
│   └── k8s/                        # 每服务多 Deployment（按 cluster）
├── go.mod
├── Makefile
└── README.md（本文件）
```

## 快速开始

### 方式 1：Docker Compose（推荐，零依赖本地运行）

```bash
cd examples/20-full-demo
make up                # 构建 + 启动 12 个容器（1 gateway + 11 业务/frontend）

# 访问
open http://localhost:8088   # 前端可视化（frontend 容器 :8088）
curl http://localhost:8080/api/services | jq   # 实例列表

# 测试路由（详见上方"端到端路由矩阵"）
curl http://localhost:8080/login
curl -H 'X-Zeus-Cluster: user.v1.1' http://localhost:8080/login

# 停止
make down
```

### 方式 2：K8s（minikube / kind）

```bash
cd examples/20-full-demo
make k8s-images       # 构建并 load 镜像
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
make build      # 编译 6 个二进制
make run        # 后台启动 1 gateway + 2 api1 + 3 srv1 + 3 srv2 + 2 srv3 = 11 实例

# 前端单独跑（Go 服务，托管静态 + 反代 /api/* 到 gateway）
PORT=8088 GATEWAY_URL=http://localhost:8080 ./bin/frontend

# 停止
make stop
```

## 端到端调用链验证

### default 链路（基线）

```bash
$ curl http://localhost:8080/login
{
  "service": "api1",
  "cluster": "default",
  "version": "v1-stable",
  "action": "api_entry",
  "downstream": {
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
}
```

### user.v1.1 链路（srv1/srv2 命中灰度，api1/srv3 降级）

```bash
$ curl -H 'X-Zeus-Cluster: user.v1.1' http://localhost:8080/login
{
  "service": "api1", "cluster": "default",  "version": "v1-stable",   "action": "api_entry",   # 降级
  "downstream": {
    "service": "srv1", "cluster": "user.v1.1", "version": "user.v1.1", "action": "user_authenticated",  # 命中
    "downstream": {
      "service": "srv2", "cluster": "user.v1.1", "version": "user.v1.1", "action": "order_created",     # 命中
      "downstream": {
        "service": "srv3", "cluster": "default", "version": "v1-stable", "action": "payment_processed"  # 降级
      }
    }
  }
}
```

### order.v2 链路（仅 srv2 命中）

```bash
$ curl -H 'X-Zeus-Cluster: order.v2' http://localhost:8080/login
# api1=default, srv1=default, srv2=order.v2, srv3=default
```

### batch.v3 链路（api1/srv1/srv3 命中，srv2 降级）

```bash
$ curl -H 'X-Zeus-Cluster: batch.v3' http://localhost:8080/login
# api1=batch.v3, srv1=batch.v3, srv2=default, srv3=batch.v3
```

每层响应里的 `cluster` 字段直观展示路由结果，证明 `X-Zeus-Cluster` 被端到端透传并按可用性降级。

## 前端可视化说明

打开 `http://localhost:8088`：
- **拓扑图**：实时显示 gateway + api1/srv1/srv2/srv3 × 各 cluster，节点显示实例数
- **按钮区**：发 default / user.v1.1 / order.v2 / batch.v3 流量，发请求时高亮对应路径
- **调用链**：嵌套 JSON 格式化展示，直观看到 4 层 cluster 命中/降级一致性

## 设计要点（KISS / DRY / SOLID 体现）

| 设计 | 体现 |
|---|---|
| **单一 Dockerfile** | `service.Dockerfile` 通过 `--build-arg SVC=...` 编译不同业务服务（DRY） |
| **服务自注册** | 各服务通过 HTTP 调 gateway 注册，无外部依赖（不需要 etcd） |
| **HTTP Discovery 适配器** | `gwdisc.New(url)` 实现 `registry.Discovery`，无缝接入 zeus client |
| **cluster 自动透传** | server/http 入口注入 ctx → client 自动透传 Header，业务代码 0 改动 |
| **cluster 降级** | client 选不到目标 cluster 实例时回退 default，灰度发布容错核心 |
| **优雅关闭** | 信号 → 反注册 → server.Stop，5s 超时兜底 |
| **frontend 即 Go 服务** | `cmd/frontend` Go 二进制托管静态 + 反代，无需 nginx 基础镜像 |

## 与 zeus 组件库的对应

| 示例组件 | zeus 包 |
|---|---|
| HTTP server | `server/http`（`httpdriver.NewHTTP`） |
| HTTP client | `client.NewClient`（自动透传 cluster + 降级） |
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

1. 把 `srv3` 改为 gRPC，用 `plugins/server/grpc`（保留 HTTP api1/srv1/srv2，实现多协议混合）
2. 集成 `ratelimit/cluster` 给 `batch.v3` 集群限流
3. 接入 etcd 替换嵌入式 memory registry（`plugins/registry/etcd`）
4. 把 frontend 的拓扑图升级为 D3.js 力导向图

---

**相关文档**：根目录 [CLAUDE.md](../../CLAUDE.md) 有 zeus 完整架构说明。
