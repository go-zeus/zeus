# zeus

[![CI](https://github.com/go-zeus/zeus/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/go-zeus/zeus/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-88.6%25-brightgreen)](https://github.com/go-zeus/zeus)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/github/license/go-zeus/zeus?color=blue)](./LICENSE)
[![Security Policy](https://img.shields.io/badge/security-policy-blue)](./SECURITY.md)
[![Contributing](https://img.shields.io/badge/contributing-guide-orange)](./CONTRIBUTING.md)
[![Discussions](https://img.shields.io/github/discussions/go-zeus/zeus?logo=github)](https://github.com/go-zeus/zeus/discussions)

零依赖、可插拔的 Go 微服务框架。现代构造器注入模式 + 内置默认装配 + 4 层渐进暴露 API。

## 设计哲学

> **内部复杂（灵活）+ 外部简单（默认）。**
>
> 用户 5 行代码启动应用，需要时能挖到底层实现细节。

详见 [CLAUDE.md - 设计目标](./CLAUDE.md#设计目标最高指导原则)

## 快速开始（L1 入口）

```go
package main

import (
    "net/http"

    "github.com/go-zeus/zeus/app"
)

func main() {
    app.Run(&app.Config{Port: 8080}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("hello from zeus"))
    }))
}

// curl http://localhost:8080
```

零配置自动启用：
- **slog logger** → stdout
- **requestid + accesslog + recovery 中间件**
- **memory 注册中心** + 自动注册
- **优雅关闭**（SIGTERM/SIGINT，10s 超时）

## 4 层渐进暴露 API

| 层 | 用户 | 入口 |
|---|---|---|
| **L1** | 学习者 / 单进程 demo | `app.Run(cfg, handler)` **5 行启动** |
| **L2** | 个人开发者 / 配置驱动 | `app.Run(cfgWithRegistry, handler)` URL scheme |
| **L3** | 小团队 / 代码定制 | `app.NewApp(opts ...AppOption)` Option 类型装配 |
| **L4** | 定制需求 / 完全控制 | `components.NewApp(comps ...any)` 声明式组件装配 |

### L2：切换注册中心 / 灰度发布

```go
app.Run(&app.Config{
    Name:     "my-service",
    Port:     8080,
    Cluster:  "canary",                    // 灰度集群
    Registry: "etcd://localhost:2379",     // 切换到 etcd（需 import 对应 plugin）
}, handler)
```

### L3：类型装配（Option）

```go
a := app.NewApp(
    app.AddServer(http.NewHTTP(http.Port(8080), http.Mux(handler))),
    app.WithRegistry(memory.New()),
    app.WithMiddleware(recovery.New()),
    app.WithServiceCluster("canary"),
)
a.Run()
```

### L4：声明式组件装配（永久逃生通道）

```go
app := components.NewApp(
    components.NewLogComponent(slog.NewSlog()),
    components.NewRegistryComponent(memory.New()),
    components.NewServerComponent(http.NewHTTP(http.Mux(mux))),
    components.NewServiceComponent(),
)
app.Run()
```

## 主要功能域

| 域 | 内置实现 | plugins 实现 |
|---|---|---|
| registry | `registry/memory` | `plugins/registry/etcd` |
| config | `config/file` | `plugins/config/etcd,k8s` |
| server | `server/http` | `plugins/server/grpc` |
| client | `client` | `plugins/client/grpc` |
| log | `log/slog` | `plugins/log/zap` |
| metrics | `metrics/noop` | `plugins/metrics/prometheus` |
| trace | `trace/noop` | `plugins/trace/otel` |
| proxy | `proxy`（HTTP/WS/SSE） | `plugins/proxy/grpc` |
| encoding | `encoding/json` | `plugins/encoding/protobuf` |
| middleware | `recovery`/`timeout`/`clustering`/`requestid`/`accesslog` | `plugins/middleware/tracing,metrics` |
| circuitbreaker | `circuitbreaker/counter`（按 cluster 隔离） | — |
| ratelimit | `ratelimit/token`（按 cluster 隔离） | — |
| retry | `retry/exponential`（按 cluster 路由） | — |
| propagation | `propagation`（W3C Baggage 兼容） | — |
| routing | `routing`（X-Zeus-Cluster 端到端路由） | — |
| job | `job/interval`（固定间隔） | `plugins/job/cron`（cron 表达式） |
| mq | `mq/memory`（进程内事件总线） | `plugins/mq/kafka`、`nats` |
| database | `database/sql`（薄封装 stdlib） | `plugins/database/mysql`/`postgres` |
| cache | `cache/memory`（TTL 双路径清理） | `plugins/cache/redis` |

### URL scheme 切换实现

各功能域支持通过 URL 字符串切换实现（与 `import _` 副作用注册配合）：

| 域 | 入口 | 已注册 scheme |
|---|---|---|
| registry | `app.resolveRegistry` | `memory` / `etcd` |
| cache | `cache.NewFromURL` | `memory` / `redis` |
| database | `database.NewFromURL` | `mysql` / `postgres` |
| mq | `mq.NewBrokerFromURL` | `memory` / `nats` |
| job | `job.NewSchedulerFromURL` | `interval` / `cron` |

## 构建

```bash
go test ./...                                  # 主包测试
go test -race -coverprofile=coverage.out ./...  # 主包覆盖率
cd examples && go build ./...                   # 示例构建
```

CI：`.github/workflows/ci.yml` — lint + test + build，覆盖主包和所有 plugins。

## 示例

- [`examples/00-app-quickstart`](./examples/00-app-quickstart) — L4 手动装配（`components.NewApp`）
- [`examples/01-hello`](./examples/01-hello) — **L1 入门（5 行）**
- [`examples/02-with-registry`](./examples/02-with-registry) — L2 多集群路由
- [`examples/03-typed`](./examples/03-typed) — **L3 类型装配**（双 Server + recovery middleware）
- [`examples/04-config-driven`](./examples/04-config-driven) — L2 配置驱动（URL scheme 切 cache/mq）
- [`examples/05-autoapp`](./examples/05-autoapp) — 自动装配最小示例
- [`examples/06-autoapp-full`](./examples/06-autoapp-full) — L4 完整装配
- [`examples/07-autoapp-multi`](./examples/07-autoapp-multi) — 多 server（HTTP + gRPC）
- [`examples/08-client`](./examples/08-client) — HTTP 客户端（自动集群路由）
- [`examples/09-middleware`](./examples/09-middleware) — 中间件链
- [`examples/10-config`](./examples/10-config) — 配置加载（file loader）
- [`examples/11-proxy`](./examples/11-proxy) — 反向代理（HTTP/WebSocket/SSE）
- [`examples/12-cluster-routing`](./examples/12-cluster-routing) — 集群路由端到端演示
- [`examples/13-database`](./examples/13-database) — 数据库抽象（事务 + tx_id 透传）
- [`examples/14-cache`](./examples/14-cache) — 缓存抽象（Set/Get/TTL）
- [`examples/15-mq`](./examples/15-mq) — 消息队列（pub/sub + baggage 透传）
- [`examples/16-job`](./examples/16-job) — 任务调度（interval 固定间隔）
- [`examples/17-job-cron`](./examples/17-job-cron) — 任务调度（cron 表达式）
- [`examples/18-propagation`](./examples/18-propagation) — Baggage 全链路传播
- [`examples/19-observability`](./examples/19-observability) — metrics + trace + log
- [`examples/20-full-demo`](./examples/20-full-demo) — 综合演示（gateway + 多 srv + 前端）
- [`examples/21-registry-etcd`](./examples/21-registry-etcd) — etcd 注册中心集成

## 社区

- 💬 [Discussions](https://github.com/go-zeus/zeus/discussions) — 提问、想法讨论、最佳实践
- 📝 [Issues](https://github.com/go-zeus/zeus/issues) — Bug 报告、功能请求
- 🤝 [Contributing](./CONTRIBUTING.md) — 贡献指南（PR 流程 / 代码规范 / 测试要求）
- 📜 [Code of Conduct](./CODE_OF_CONDUCT.md) — 行为准则
- 🔒 [Security Policy](./SECURITY.md) — 安全漏洞披露流程
- 📚 [设计文档](./CLAUDE.md) — 4 层 API 设计哲学、概念分层、组件装配
- 🌐 [文档站](./site/content/) — Hugo 源（`hugo server` 本地预览 / 推 main 自动部署到 GitHub Pages）
- 🔧 [API 稳定性](./site/content/reference/api-stability.md) — 🔒稳定 / 🧪实验 / 🔬内部分级

## 治理

| 文件 | 用途 |
|---|---|
| [CHANGELOG.md](./CHANGELOG.md) | 每版本变更记录（breaking / feature / fix） |
| [SECURITY.md](./SECURITY.md) | 漏洞披露政策与响应 SLA |
| [site/content/reference/api-stability.md](./site/content/reference/api-stability.md) | API 稳定性分级清单 |
| [site/content/reference/plugin-bom.md](./site/content/reference/plugin-bom.md) | 插件依赖版本治理策略 |
| [site/content/reference/optimization-plan.md](./site/content/reference/optimization-plan.md) | 业界标杆对比 + 路线图 |

## 致谢

Zeus 的设计参考了以下优秀框架，但未直接复制代码：

- [kratos](https://github.com/go-kratos/kratos) — BFF / 微服务架构思路
- [go-zero](https://github.com/zeromicro/go-zero) — 工程化与代码生成
- [gin](https://github.com/gin-gonic/gin) — 路由与中间件链
- [kitex](https://github.com/cloudwego/kitex) — RPC 治理与扩展点
- [Go 标准库](https://pkg.go.dev/std) — `net/http` / `database/sql` / `log/slog`

详见 [CLAUDE.md - 设计参考](./CLAUDE.md#设计参考不是抄作业)。

## 许可证

[MIT](./LICENSE)

