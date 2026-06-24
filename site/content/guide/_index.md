---
title: 用户指南
weight: 20
---

按功能域组织。每个功能域统一结构：

```
功能域/
├── 功能域.go          ← 接口定义 + 用户 API
├── 内置实现/           ← 零依赖，导出 New() 构造函数
└── (plugins/第三方实现) ← 有第三方依赖，独立 go.mod
```

## 功能域清单

| 域 | 内置实现 | plugins 实现 |
|---|---|---|
| [registry](registry) | `registry/memory` | `plugins/registry/etcd` / `nacos` |
| config | `config/file` | `plugins/config/etcd,k8s` |
| server | `server/http` | `plugins/server/grpc` |
| [client](client) | `client` | `plugins/client/grpc` |
| log | `log/slog` | `plugins/log/zap` / `file_rotate` |
| [observability](observability) (metrics/trace/log) | `metrics/noop` / `trace/noop` / `log/slog` | `plugins/metrics/prometheus` / `plugins/trace/otel` |
| [proxy](proxy) | `proxy`（HTTP/WS/SSE） | `plugins/proxy/grpc` |
| encoding | `encoding/json` | `plugins/encoding/protobuf` |
| [middleware](middleware) | `recovery`/`timeout`/`clustering`/`requestid`/`accesslog` | `plugins/middleware/tracing,metrics` |
| circuitbreaker | `circuitbreaker/counter`（按 cluster 隔离） | — |
| ratelimit | `ratelimit/token`（按 cluster 隔离） | — |
| retry | `retry/exponential`（按 cluster 路由） | — |
| [propagation](propagation) | `propagation`（W3C Baggage 兼容） | — |
| [cluster-routing](cluster-routing) | `routing`（X-Zeus-Cluster 端到端路由） | — |
| [job](job) | `job/interval`（固定间隔） | `plugins/job/cron`（cron 表达式） |
| [mq](mq) | `mq/memory`（进程内事件总线） | `plugins/mq/nats,kafka` |
| [database](database) | `database/sql`（薄封装 stdlib） | `plugins/database/mysql`/`postgres`/`sqlite` |
| [cache](cache) | `cache/memory`（TTL 双路径清理） | `plugins/cache/redis` |

## URL scheme 切换

各功能域支持 URL scheme 切换实现：

| 域 | 入口 | 已注册 scheme |
|---|---|---|
| registry | `app.resolveRegistry` | `memory` / `etcd` / `nacos` |
| cache | `cache.NewFromURL` | `memory` / `redis` |
| database | `database.NewFromURL` | `mysql` / `postgres` / `sqlite` |
| mq | `mq.NewBrokerFromURL` | `memory` / `nats` / `kafka` |
| job | `job.NewSchedulerFromURL` | `interval` / `cron` |

注册机制：用户主程序 `import _ "..."` 副作用包即可激活对应 scheme（plugins 在 `init()` 中注册）。
