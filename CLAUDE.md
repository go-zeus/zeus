# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Zeus 是一个零依赖、可插拔的 Go 微服务框架。采用现代 Go 构造器注入模式：功能域主包定义有意义的接口和用户 API，内置实现直接返回接口类型，第三方实现放在 `plugins/` 下（独立 go.mod）。

## 设计目标（最高指导原则）

### 核心哲学

> **内部复杂（灵活）+ 外部简单（默认）。**
>
> 用户 5 行代码启动应用，需要时能挖到底层实现细节。

所有 PR、API 设计、文档组织都必须服从这条哲学。新增功能前先回答："能不能在用户不需要时看不到？"

### 4 层渐进暴露 API

每一层覆盖一类用户，**不允许越层泄漏概念**。

| 层 | 用户 | 入口 | 暴露概念 | 目标 |
|---|---|---|---|---|
| **L1** | 学习者 / 单进程 demo | `zeus.Run(cfg, handler)` | 仅 `App` + `Config` | 5 行启动 |
| **L2** | 个人开发者 / 配置驱动 | `zeus.Run(cfgWithRegistry, handler)` | `App` + `Config` + `Registry`（URL 字符串） | 改配置文件即可 |
| **L3** | 小团队 / 代码定制 | `app.NewApp(opts...)`（Option 类型装配） | `Server` + `Logger` + `Registry` + `Middleware` + ... | 类型装配 |
| **L4** | 定制需求 / 完全控制 | `components.NewApp(comps...)` | 全部组件接口 | 永久逃生通道 |

**层级约束**：
- L1 用户**不应感知** Component / Container / Lifecycle / Instance / ServiceEntry / Cluster 等内部概念
- L2 用 URL scheme（`etcd://` / `memory://` / `k8s://`）代替直接 import 实现包
- L3 是当前 `components` 包的"扁平化版本"，去 Component 包装
- L4 完整保留，作为高级逃生通道

### 默认装配清单（L1/L2 开箱即用）

**未配置时自动启用**，用户零感知：

| 默认项 | 内置实现 |
|---|---|
| Server 协议选择 | 按 handler 类型推断（`http.Handler` → HTTP，`*grpc.Server` → gRPC） |
| 注册中心 | `registry/memory`（L1）/ 用户指定 URL（L2+） |
| 日志 | `log/slog`（输出到 stdout） |
| 中间件 | recovery + request ID + 请求日志 |
| 健康检查 | `/health` `/health/ready` `/health/live` |
| Metrics | `/metrics`（noop meter 默认，可注入 prometheus） |
| 信号处理 | SIGTERM/SIGINT/SIGQUIT → 优雅关闭（10s 超时） |
| 服务名 | 默认 `zeus-service`（用户可覆盖） |

**关键规则**：默认装配**不允许失败**。任何"必须配置才能跑"的字段都是设计缺陷。

### 禁止规则（防止概念泄漏）

1. **L1/L2 文档不能出现** `Component` / `Container` / `Lifecycle` / `Provide` / `Instance` / `ServiceEntry` / `Watcher` / `Loader` / `Interceptor` 等内部接口名
2. **L1 必须支持 0 配置启动**，没有"必填字段"
3. **新增功能先评估能否做默认**，再考虑做成 Option
4. **类型推断优于显式选择**（handler 类型决定 server 协议，而非 `WithHTTPServer()`）
5. **配置项命名跟用户心智对齐**：`Port` / `Name` / `Registry` —— 不是 `ServerOptions.Addr` / `ServiceConfig.Name`

### 验收指标（可量化）

| 指标 | 目标 | 业界标杆 |
|---|---|---|
| Hello world 代码行数 | ≤ 5 行 | Gin: 7 行 / FastAPI: 5 行 |
| L1 用户需记忆的概念数 | ≤ 2 个（App + Config） | Redis: 5 / Gin: 3 |
| 默认装配数量 | ≥ 8 个组件 | Django: batteries included |
| 从 demo 到生产改动 | 仅改配置 | go-zero: etc/ |
| L1 → L4 切换成本 | 渐进，无需重写 | Spring Boot 分层 |

### 演进策略

当前框架处于 **L1/L2/L3/L4 全部完整阶段**。演进路径：

1. **第一步（已完成）**：在 `app/` 包定义 `zeus.Run(cfg, handler)` 入口（包装 L4 默认装配） — `app/quickstart.go`
2. **第二步（已完成）**：定义 `app.Config` 结构体（仅暴露 L1/L2 字段）+ 类型驱动的 Server 选择（按 handler 类型自动装配）
3. **第三步（已完成）**：实现 Registry URL scheme 解析（`etcd://` / `k8s://` / `memory://`） — `app/quickstart.go` + `app/app.go`
4. **第四步（已完成）**：补充默认中间件链（recovery/requestID/log/health/metrics 自动注入） — `app/quickstart.go`
5. **第五步（已完成）**：实现 L3 类型装配入口 `app.NewApp(opts ...any) *components.App`（扁平化 Option 模式） — `app/options.go`
   - 关键设计：L3 是 L4 的"语法糖"，返回 `*components.App`，底层 100% 复用 Container/Lifecycle
   - 混用支持：`app.NewApp(...)` 参数末尾可直接追加 `components.NewXxxComponent(...)` 实现渐进升级
   - Option 清单：`AddServer` / `WithLogger` / `WithRegistry` / `WithMeter` / `WithTracer` / `WithMiddleware` / `WithServiceName` / `WithServiceCluster` / `WithServiceIP` / `WithStopTimeout` / `WithComponent`
6. **第六步（已完成）**：扩展 URL scheme 覆盖到 cache/database/mq（与 registry 行为一致）
   - 各功能域建立 `RegisterResolver` + `NewFromURL` 框架（`cache/resolver.go` / `database/resolver.go` / `mq/resolver.go`）
   - plugins 在 `init()` 中注册自己的 scheme（如 `redis://` / `mysql://` / `kafka://`）
   - L2 `Config` 增加可选 URL 字段（`Cache` / `Database` / `MQ`），保持零配置启动
   - L3 增加 `WithCacheURL` / `WithDatabaseURL` / `WithMQURL` Option

**约束**：演进过程中 L4 API（当前 `components.NewApp`）必须保持兼容，作为永久逃生通道。

### 设计参考（不是抄作业）

| 项目 | 借鉴点 | 不抄 |
|---|---|---|
| Redis | 5 个核心数据结构撑起整个生态 | 不限制接口数量 |
| Gin | 1 个路由 + 1 个中间件链 = 80% 业务 | 不做 router-centric |
| FastAPI | 类型即文档、装饰器即路由 | 不引入 Python 风格 |
| Django | "batteries included" 默认齐全 | 不做 admin 后台 |
| go-zero | 代码生成消灭样板 | 不做 .api DSL |
| gRPC | 一个 .proto → 自动生成 | 不引入 IDL |

## 概念分层（业务 vs 运行时 vs 注册中心）

为避免混淆，明确各层职责：

| 层 | 概念 | 归属 | 说明 |
|---|---|---|---|
| 业务层 | `UserService` / `OrderService` / `Agent` | **用户自治** | 框架不感知业务 service/agent，由用户用 Go struct 表达 |
| 运行时层 | `App` | 框架 | 进程级，管信号 + 优雅关闭 + 持有 1..N 个 Server |
| 运行时层 | `Server` | 框架 | Endpoint = ip:port，对应一种协议（HTTP/gRPC/...） |
| 注册中心层 | `ServiceEntry` | 框架（对齐 K8s/Istio） | 同名 Instance 的逻辑集合 |
| 注册中心层 | `Instance` | 框架（对齐 Endpoints） | 一个 Server 一个 Instance，带 Protocol 字段 |
| 注册中心层 | `Cluster` | 框架 | 实例按 Cluster 字段聚合（路由 key） |

**核心规则**：
- 一个 `App` 可持有多个 `Server`（HTTP + gRPC 同进程）
- 每个 `Server` 对应一个注册中心 `Instance`（共享 Name，区分 Protocol）
- 注册/反注册最小单位是 `Instance`（不是 Server 或业务 service）
- 业务 service（UserService 等）不参与注册，挂到 Server 的 Handler 即可

**可见性规则（呼应设计目标）**：
- 本节所有概念（`App`/`Server`/`Instance`/`ServiceEntry`/`Cluster`）属于 **L3 及以上** 用户可见
- L1/L2 用户不应感知 `Instance` / `ServiceEntry` / `Cluster`，仅看到 `Service` 这个聚合词
- 在文档与 API 命名时，"用户感知概念"和"内部实现概念"必须分层标注

## 构建与测试

```bash
go test ./...
go test -race -coverprofile=coverage.out ./...
go test ./registry/...
go vet ./...
```

### Go 版本分层设计（重要，避免误改）

仓库中存在三个不同 Go 版本声明，**这不是错误，是有意设计**：

| 文件 | Go 版本 | 角色 | 原因 |
|---|---|---|---|
| 主仓 `go.mod` | `go 1.22` | 用户兼容性下限 | 框架使用者最低 Go 版本承诺（log/slog 等标准库 API 要求） |
| `examples/21-registry-etcd/go.mod` | `go 1.25` | etcd 依赖要求 | etcd v3.6.x 强制要求 Go 1.25+，无法降级 |
| `examples/19-observability/go.mod` | `go 1.23` | otel 依赖要求 | otel v1.24 要求 Go 1.23+ |
| `go.work` | `go 1.25.0` | 开发期 workspace | **必须取所有模块最大值**，否则 `go build/work sync` 报 "module requires go >= 1.25.0" |

**不要**为了"统一"把主仓 `go.mod` 提到 1.25（会损失向下兼容性），也**不要**把 `go.work` 降到 1.22（编译失败）。

### 模块结构（开发期 workspace）

项目使用 Go workspace（`go.work`），所有 examples 模块独立 `go.mod`，通过 `replace` 指令引用本地 zeus。`go.work` 注册全部 22 个 examples 子目录用于本地联调，发版时每个 example 可独立 `go mod tidy` 后被用户复制使用。

plugins/* 也是独立 module，**不**在 go.work 中注册（破坏面过大，需各自在目录下 `GOWORK=off go test ./...` 验证）。

CI：`.github/workflows/ci.yml` — lint + test + coverage，Go 1.22（主仓）+ Go 1.25（workspace）。

## 架构设计

### 接口定义模式

每个功能域遵循统一结构：

```
功能域/
├── 功能域.go          ← 接口定义 + 用户 API
├── 内置实现/           ← 零依赖，同一 go.mod，导出 New() 构造函数
└── (plugins/第三方实现) ← 有第三方依赖，独立 go.mod
```

| 功能域 | 接口名 | 用户 API | 内置实现 | plugins 实现 |
|--------|--------|----------|----------|-------------|
| registry | `Registrar`/`Discovery`/`Watcher` | 纯接口 | `registry/memory` | `plugins/registry/etcd` |
| balancer | `Balancer` | 纯接口 | `balancer/random,round_robin` | — |
| server | `Server` | 纯接口 | `server/http`（含健康检查 + 自动集群路由注入） | `plugins/server/grpc`（含自动集群路由注入） |
| ~~service~~ | — | **已删除**（职责与 app/components 重叠） | — | — |
| log | `Writer` | `Logger` 结构体（With/Close） | `log/slog`（+ stdWriter 自动注入 cluster Field） | `plugins/log/zap` |
| config | `Loader`/`Watcher`/`Decoder` | `Config` 结构体（Get/Watch/Close） | `config/file` | `plugins/config/etcd,k8s`（etcd = KV 配置树；k8s = ConfigMap 加载） |
| encoding | `Codec` | 纯接口 | `encoding/json` | `plugins/encoding/protobuf` |
| middleware | `Interceptor` | `Chain` 类型 | `middleware/recovery,timeout,clustering` | `plugins/middleware/tracing,metrics`（自动注入 cluster） |
| circuitbreaker | `Breaker` / `cluster.ClusterBreaker` | `CircuitBreaker` 结构体（Execute） | `circuitbreaker/counter`、按 cluster 隔离 | — |
| ratelimit | `Limiter` / `cluster.ClusterLimiter` | 纯接口 | `ratelimit/token`、按 cluster 隔离 | — |
| retry | `Retrier` / `cluster.ClusterRetrier` | 纯接口 | `retry/exponential`、按 cluster 路由 | — |
| metrics | `Meter`/`Counter`/`Histogram`/`Gauge` | 纯接口 | `metrics/noop` | `plugins/metrics/prometheus` |
| trace | `Tracer`/`Span` | 纯接口 | `trace/noop` | `plugins/trace/otel` |
| proxy | `Proxy`（实现 `http.Handler`）+ `Selector` | `proxy.New(WithSelector)` | `proxy`（HTTP/WebSocket/SSE 内置） | `plugins/proxy/grpc` |
| routing | — | `routing.WithCluster`/`FromContext`/`ClusterFromHTTPHeader` | `routing`（HTTP Header + gRPC metadata 统一抽象，对齐 K8s/Istio cluster 概念，底层基于 propagation） | — |
| propagation | `Bag`/`Entry` | `propagation.With`/`Get`/`InjectHTTP`/`ExtractHTTP` | `propagation`（W3C Baggage 兼容的 K-V 上下文传播，零依赖） | — |
| job | `Scheduler` + `Spec` | `job.Spec{Name, Every, Handler}` + `interval.New()` | `job/interval`（基于 time.Ticker 的固定间隔，零依赖） | `plugins/job/cron`（cron 表达式，基于 robfig/cron） |
| mq | `Publisher`/`Subscriber`/`Broker` + `Message`/`Handler` | `mq.Message{Topic, Payload, Headers}` + `memory.New()` | `mq/memory`（基于 channel fan-out，无缓冲反压，零依赖） | `plugins/mq/kafka`/`nats`/`redis` 等（待补） |
| database | `DB`/`Tx`/`Rows`/`Row` | `database.DBOptions` + `database.WithTx(ctx, tx)`/`FromTx(ctx)` | `database/sql`（薄封装 stdlib database/sql，自动 trace/metrics/tx_id） | `plugins/database/mysql`（postgres/memcached 待补） |
| cache | `Cache` | `cache.Item{Key, Value, TTL}` + `cache.WithTTL(d)` | `cache/memory`（基于 sync.Map + TTL 双路径清理，零依赖） | `plugins/cache/redis`（memcached 待补） |
| client | `HTTPClient`（`type Client = HTTPClient` 别名兼容） | `client.NewClient`（HTTP 专用，自动集群路由 + baggage 传播） | `client` | `plugins/client/grpc`（独立抽象，自动注入 cluster metadata + baggage） |

### 构造与使用

```go
import (
    "github.com/go-zeus/zeus/registry/memory"
    "github.com/go-zeus/zeus/server/http"
    "github.com/go-zeus/zeus/log/slog"
)

// 直接构造，构造器注入
reg := memory.New()
srv := http.NewHTTP()
logger := log.NewLogger(slog.NewSlog())
```

### 核心流程

**手动模式：**
```
engine.go → app.New() → app.Run()
  └─ 多 server 并发启动（errgroup）
  └─ 信号监听（SIGTERM/SIGINT/SIGQUIT）
  └─ context 取消 → server.Stop() 优雅关闭（10s 超时）
```

**自动装配模式（推荐）：**
```
engine.go → components.NewApp(comps...) → app.Run()
  └─ 拓扑排序解析依赖
  └─ 按序调用 Provide → OnStart
  └─ 信号监听 → 逆序 OnStop 优雅关闭
```

## L3 类型装配（app.NewApp + WithXxx Option 模式）

`app.NewApp(opts ...any) *components.App` 是 L4 的扁平化 Option API 包装：

- **入口**：`app.NewApp(opts...)` — 类型装配，用户用 `AddServer(s)` / `WithLogger(l)` 等 Option
- **返回 `*components.App`**（L3 与 L4 同类型，零适配成本）
- **底层 100% 复用 L4**：Container / Lifecycle / 拓扑排序全部沿用，不绕过任何 L4 能力
- **L3/L4 混用**：`NewApp(...)` 参数末尾可直接追加 `components.NewXxxComponent(...)` 实现渐进升级

### Option 清单

| Option | 签名 | 默认值 | 说明 |
|---|---|---|---|
| `AddServer(s)` | 追加 | 必填至少 1 个 | 多次调用累加（多 Server 多 Instance 场景） |
| `WithLogger(w)` | 覆盖 | `slog.NewSlog()`（stdout） | 装 `LogComponent` |
| `WithRegistry(r)` | 覆盖 | `memory.New()` | 装 `RegistryComponent`（不做 URL scheme 解析） |
| `WithMeter(m)` | 覆盖 | 不装（noop 兜底） | 装 `MetricsComponent` |
| `WithTracer(t)` | 覆盖 | 不装 | 装 `TraceComponent` |
| `WithMiddleware(mw)` | 追加 | 空 | 每个独立 `MiddlewareComponent`（Name=`middleware_<mw.Name()>`） |
| `WithServiceName(s)` | 覆盖 | `"zeus-service"` | 透传 `ServiceComponent` |
| `WithServiceCluster(s)` | 覆盖 | `"default"` | 同上 |
| `WithServiceIP(s)` | 覆盖 | 自动探测 | 同上 |
| `WithStopTimeout(d)` | 覆盖 | `10s` | 透传 `components.WithStopTimeout` |
| `WithComponent(c)` | 追加 | — | L3/L4 混用：透传任意 L4 Component 或 AppOption |
| `WithCacheURL(url)` | 覆盖 | 不装 | URL 字符串装配 cache（如 `"memory://?name=x"` / `"redis://..."`） |
| `WithDatabaseURL(url)` | 覆盖 | 不装 | URL 字符串装配 database（自动透传 tracer/meter） |
| `WithMQURL(url)` | 覆盖 | 不装 | URL 字符串装配 mq broker |

### URL scheme 注册矩阵

各功能域独立的 `RegisterResolver` / `NewFromURL` 框架，与 registry URL scheme 行为一致：

| 功能域 | 入口函数 | 已注册 scheme | 实现 |
|---|---|---|---|
| registry | `app.NewFromURL`（实际在 `app.resolveRegistry`） | `memory` / `etcd`（plugin） | `registry/memory` / `plugins/registry/etcd` |
| cache | `cache.NewFromURL` | `memory` / `redis`（plugin） | `cache/memory` / `plugins/cache/redis` |
| database | `database.NewFromURL(url, tracer, meter)` | `mysql`（plugin） | `plugins/database/mysql`（postgres 待补） |
| mq | `mq.NewBrokerFromURL` | `memory` | `mq/memory`（kafka/nats 待补） |
| job | `job.NewSchedulerFromURL` | `interval` / `cron`（plugin） | `job/interval` / `plugins/job/cron` |

注册机制：用户在主程序 `import _ "..."` 副作用包即可激活对应 scheme（plugins 在 `init()` 调用 `RegisterResolver`）。主仓零依赖。

### 与 L1 的关键差异

- L1 自动包装 recovery/requestID/log/health/metrics 中间件；L3 **不自动包装**
- 原因：L3 用户已直接构造 `http.NewHTTP()`，对 server 中间件链有完全控制
- 默认链需用户显式：`WithMiddleware(recovery.New())`

### 用法示例

```go
import (
    "github.com/go-zeus/zeus/app"
    "github.com/go-zeus/zeus/middleware/recovery"
    "github.com/go-zeus/zeus/registry/memory"
    "github.com/go-zeus/zeus/server/http"
)

a := app.NewApp(
    app.AddServer(http.NewHTTP(http.Port(8080), http.Mux(handler))),
    app.WithRegistry(memory.New()),
    app.WithMiddleware(recovery.New()),
    app.WithServiceName("my-app"),
    app.WithServiceCluster("canary"),
)
a.Run()
```

### L3/L4 混用（关键卖点）

用户从 L3 平滑升级到 L4：在 `NewApp(...)` 参数末尾追加任意 L4 Component：

```go
a := app.NewApp(
    app.AddServer(http.NewHTTP()),
    app.WithMiddleware(recovery.New()),

    // 直接追加 L4 组件（无需 WithComponent 包装）
    components.NewCacheComponent(myCache),
    components.NewJobComponent(scheduler),
    components.NewMQSubscription("topic", handler),
)
```

完整示例参见 `examples/03-typed/`：双 Server + recovery + memory registry。

## 组件自动装配（components 包）

`components` 包提供声明式组件组装 + 生命周期编排：

- **Component 接口**：`Name()/Depends()/Provide(ctx)/Lifecycle()` — 组件声明
- **Container**：注册组件 → 拓扑排序 → 按序启动 → 逆序停止
- **App**：Container + 信号监听 + 优雅关闭

```go
import (
    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/config/file"
    "github.com/go-zeus/zeus/log/slog"
    "github.com/go-zeus/zeus/server/http"
)

app := components.NewApp(
    components.NewLogComponent(slog.NewSlog()),
    components.NewServerComponent(),
    components.NewServiceComponent(),
)
app.Run()
```

所有组件适配器接受实例注入（不接受字符串名称）。

内置适配器：Registry/Server/Config/Log/Metrics/Trace/CircuitBreaker/RateLimit/Retry/Middleware/Service
通用适配器：`components.Adapt("name", instance)` 快速包装任意实例

### ServerComponent / ServiceComponent 多 Server 模型

```go
// 单 server（默认 HTTP :8080）
components.NewServerComponent()
// 单 server（显式 HTTP，自定义配置）
components.NewServerComponent(http.NewHTTP(http.Port(9001)))
// 多 server（HTTP + gRPC 同进程）
components.NewServerComponent(http.NewHTTP(...), grpc.NewGRPC(...))

// ServiceComponent 为每个 Server 生成一个 Instance（带 protocol 字段）
components.NewServiceComponent(
    components.WithServiceName("my-app"),
    components.WithServiceCluster("canary"),
)
// → 注册 1..N 个 Instance（每个 server 一个，共享 Name，区分 Protocol）
```

ServiceComponent 在 OnStart 时遍历所有 Instance 调用 `Registrar.Register`，OnStop 时调用 `Registrar.Deregister`。优雅关闭逆序进行。

## 健康检查

server/http 内置健康检查端点（使用 DefaultHandler 时自动注册）：
- `GET /health` — 固定返回 200
- `GET /health/ready` — 调用 HealthChecker.IsReady()
- `GET /health/live` — 调用 HealthChecker.IsAlive()

## 数据模型

注册中心三层模型（对齐 K8s Endpoints + Istio ServiceEntry）：

- `types.Instance` — 实例（一个进程的一个端口），含 `Id/Name/Cluster/Protocol/Ip/Port/Metadata/Labels`
- `types.Cluster` — 集群（同名+同 cluster 的实例集合，按 cluster 路由的候选池）
- `types.ServiceEntry` — 逻辑服务（同名实例的集合，二级索引：`Instances map[id]*Instance` + `Clusters map[name]*Cluster`）

命名说明：`ServiceEntry` 替代旧 `Service` 名称（消除与业务 service 概念的冲突）。保留 `type Service = ServiceEntry` alias 一个版本。

注册/反注册最小单位是 `*types.Instance`。多协议应用注册多条 Instance（每条带 `Protocol` 字段，如 `http`/`grpc`）。

## 集群路由（端到端 X-Zeus-Cluster）

并行开发场景下，多个项目共享同一套服务，每个项目对应一组灰度实例（cluster）。流量通过 `X-Zeus-Cluster` Header 端到端路由：**"有标识走标识，无标识走 default"**。术语统一为 `cluster`，与 K8s/Istio/Envoy/gRPC xDS 对齐。

### 传播链路

```
[Client] X-Zeus-Cluster: canary
   │
   ▼ HTTP Header / gRPC metadata["x-zeus-cluster"]
[Gateway (proxy)] NewDiscoverySelector 从 Header 读 cluster → 选 cluster → 转发
   │
   ▼ Header 透传
[HTTP Server (srv-1)] 入口 clusterInjector 自动注入 ctx
   │   ├─ tracing: span.Attrs["zeus.cluster"]=cluster
   │   ├─ metrics: labels["cluster"]=cluster
   │   └─ log: 自动 Field{cluster}
   ▼
[业务 handler] ctx 已含 cluster
   │
   ▼ client.Do(): resolveCluster 读 ctx → 选 cluster → 注入 Header
[HTTP Server (srv-2)] 同 srv-1
```

### 核心 API

```go
import "github.com/go-zeus/zeus/routing"

// HTTP 入口注入
ctx := routing.WithCluster(r.Context(), routing.ClusterFromHTTPHeader(r.Header))

// 业务读取
c := routing.FromContext(ctx)

// 常量
routing.HeaderCluster   // "X-Zeus-Cluster"
routing.MetadataCluster // "x-zeus-cluster"（gRPC metadata）
routing.Default         // "default"
```

### 默认行为

- **server/http**：默认自动注入 cluster（`WithoutAutoClustering()` 关闭）
- **plugins/server/grpc**：默认 UnaryServerInterceptor 从 metadata 提取 cluster 注入 ctx
- **plugins/client/grpc**：`UnaryInterceptor()` 从 ctx cluster 注入 outgoing metadata
- **log**：自动 prepend `Field{cluster}`（仅非 default 时）
- **plugins/middleware/tracing**：自动写入 span attribute `zeus.cluster`
- **plugins/middleware/metrics**：自动打 label `cluster`

### 治理模块按 cluster 维度

```go
import clusterlimit "github.com/go-zeus/zeus/ratelimit/cluster"
import clusterbreak "github.com/go-zeus/zeus/circuitbreaker/cluster"
import clusterretry "github.com/go-zeus/zeus/retry/cluster"

// 每个 cluster key 独立桶/熔断器/重试策略
limiter := clusterlimit.New(func() ratelimit.Limiter { return token.New(100, 10) })
ok := limiter.Allow(ctx) // 从 ctx 提取 cluster 作为 key

cb := clusterbreak.New(func() circuitbreaker.Breaker { return counter.New(100, 0.5) })
err := cb.Execute(ctx, func() error { ... })

cr := clusterretry.New(func() retry.Retrier { return exponential.New(3, 100*time.Millisecond) })
r := cr.NewRetriever(ctx)
```

### 完整示例

参见 `examples/cluster_routing/`：单进程演示 gateway → srv1 → srv2 多 cluster 路由 + cluster 全链路传播。

启动后测试：
```bash
curl http://localhost:8081/ping                                # default 链路
curl -H "X-Zeus-Cluster: canary" http://localhost:8081/ping    # canary 链路
```

## 可观测性（metrics + trace + log）

三件套联动模式：构造期注入 → 中间件链自动埋点 → 优雅关闭时 batch flush。

```go
import (
    "github.com/go-zeus/zeus/middleware"
    "github.com/go-zeus/zeus/middleware/recovery"
    metricsmw "github.com/go-zeus/zeus/plugins/middleware/metrics"
    tracingmw "github.com/go-zeus/zeus/plugins/middleware/tracing"
    "github.com/go-zeus/zeus/plugins/metrics/prometheus"
    "github.com/go-zeus/zeus/plugins/trace/otel"
)

meter := prometheus.New(prometheus.WithNamespace("zeus"))
tracer := otel.New(otel.WithServiceName("my-app"))

// 链顺序：recovery → tracing → metrics（外→内）
chain := middleware.NewChain(recovery.New(), tracingmw.New(tracer), metricsmw.New(meter))

// /metrics 端点：mux.Handle("/metrics", prometheus.HTTPHandler())
// 优雅关闭时 TraceComponent.OnStop 调用 tracer.Close()，OTel batch flush 完成
```

tracing 中间件自动注入 `zeus.cluster` span attribute（仅非 default）；metrics 中间件按 `cluster + method + status` 三维 label 计数 `zeus_requests_total`。

完整端到端示例参见 `examples/observability/`。

## 反向代理网关（proxy 包）

`proxy` 包提供多协议反向代理，统一 `http.Handler` 入口按协议自动嗅探分流：

```go
import (
    "net/http"
    "net/url"
    "github.com/go-zeus/zeus/proxy"
    "github.com/go-zeus/zeus/balancer/round_robin"
)

// 静态模式
target, _ := url.Parse("http://127.0.0.1:9000")
p := proxy.New(proxy.WithSelector(proxy.NewStaticSelector(target)))
http.ListenAndServe(":8081", p)

// 动态模式（服务发现 + 集群路由）
p := proxy.New(proxy.WithSelector(
    proxy.NewDiscoverySelector("api-svc", dis, round_robin.New()),
))
```

支持的协议：
- **HTTP/HTTPS**：基于 `httputil.ReverseProxy`，自动注入 `X-Forwarded-For`/`X-Real-IP`/`X-Request-ID`
- **WebSocket**：Hijack + raw io.Copy 透传（nginx 风格，不解析 RFC6455 帧）
- **SSE**：禁用缓冲 + Flusher，串行 read-write-flush 保证事件顺序
- **gRPC**：走独立 plugin 模块 `plugins/proxy/grpc`，独立监听端口（HTTP/2 多路复用）

`Selector` 接口抽象后端选择：
- `NewStaticSelector(target *url.URL)` — 固定后端
- `NewDiscoverySelector(name, dis, lb)` — 动态服务发现 + 负载均衡 + 集群路由

扩展点：`WithDirector`/`WithResponseRewriter`/`WithErrorHandler`/`WithTransport`

## plugins 目录

`plugins/` 下每个子目录是独立 Go module（有自己的 go.mod），可有第三方依赖。开发时用 `replace` 指向本地核心，发版时删除 replace。统一版本号管理。

## 代码规范

- 注释语言与现有代码保持一致（中文注释）
- Go 版本：1.22+（log/slog 需要）
- 新增功能域在主包定义有意义的接口名，实现包导出 `New()` 构造函数
- 内置实现必须零第三方依赖，有依赖的放 plugins/
- 所有新代码必须有对应测试

## 通用上下文传播（propagation 包）

`propagation` 包提供 W3C Baggage 兼容的跨进程 K-V 上下文传播，扩展自单一 `X-Zeus-Cluster` Header，支持用户自定义任意 K-V（如 `tenant.id` / `feature.flag` / `region`）的全链路透传。

### 与 routing 的关系

| 维度 | routing | propagation |
|------|---------|-------------|
| 范围 | 仅 `zeus.cluster` 单字段 | 任意 K-V |
| 协议 | `X-Zeus-Cluster` Header / gRPC metadata | `Baggage` Header（W3C 标准） |
| 实现 | 基于 propagation，同步到 Bag | W3C baggage 编解码 + Bag/Entry 数据结构 |

`routing.WithCluster` 同时写入 ctx 本地值 + propagation Bag，`routing.FromContext` 优先读 ctx 本地值，缺失时从 Bag 兜底。两侧保持一致。

### 自动传播矩阵（用户零感知）

| 位置 | 行为 |
|------|------|
| `server/http` 入口 | `clusterInjector` 自动 `ExtractHTTP` 注入 ctx |
| `plugins/server/grpc` 入口 | `clusterInterceptor` 自动 `ExtractMetadataMulti` |
| `client.Do` 出口 | 自动 `InjectHTTP` 写入 `Baggage` Header |
| `plugins/client/grpc` 出口 | `UnaryInterceptor` 自动 `InjectMetadataMulti` |
| `proxy` 反向代理 | HTTP Header 自然透传（`httputil.ReverseProxy` 默认行为） |
| `log` 包 | 自动从 ctx 读 baggage entries 写成 Field |
| `plugins/middleware/tracing` | 自动写 span attribute（每个 K-V 一个） |
| `plugins/middleware/metrics` | 默认不加 baggage label（避免基数爆炸），用户通过 `WithBaggageLabels` 显式声明 |

### 核心 API

```go
import "github.com/go-zeus/zeus/propagation"

// 业务代码注入 K-V（一次性）
ctx = propagation.With(ctx, "tenant.id", "acme")
ctx = propagation.With(ctx, "feature.flag", "beta")

// 业务代码读取
v, ok := propagation.Get(ctx, "tenant.id")

// 手动注入/提取（仅在不走 zeus client/server 时需要）
propagation.InjectHTTP(ctx, req.Header)
ctx = propagation.ExtractHTTP(ctx, r.Header)
```

### 不自动传播的场景

绕过 Zeus 抽象时需手动调用：

- 直接用 `net/http.Client.Do()` → 手动 `propagation.InjectHTTP(ctx, req.Header)`
- 直接用 `grpc.Dial()` → 手动 `propagation.InjectMetadataMulti(ctx, md)`
- 直接用 `kafka-go` / `sarama` 等 MQ 库 → 在消息 Header 中手动写入/读取

### 完整示例

参见 `examples/propagation/`：演示 baggage 入站自动 extract + 业务注入 + log 自动带 Field。

## 任务调度（job 包）

`job` 包提供声明式周期性任务调度抽象。主包定义 `Scheduler` 接口 + `Spec` 结构，内置 `interval` 实现基于 `time.Ticker`（零依赖），高级 cron 表达式走 `plugins/job/cron`。

### 核心概念

| 概念 | 说明 |
|------|------|
| `Spec` | 任务规格（Name + Schedule/Every + Handler + Timeout） |
| `Scheduler` | 调度器接口（Register/Start/Stop） |
| `interval.Scheduler` | 内置实现：固定间隔，每 Job 独立 goroutine + Ticker |
| `JobComponent` | components 适配器：声明式注册 + 自动启停 |
| `JobRegistration` | 单个 Job 包装为组件 |

### 设计权衡

| 维度 | 选择 | 理由 |
|------|------|------|
| 内置调度器 | `time.Ticker` 固定间隔 | 零依赖、覆盖 80% 用例（心跳/上报/清理） |
| Cron 表达式 | 放 `plugins/job/cron` | cron 解析复杂，且需要 robfig/cron 依赖 |
| 首次执行 | 立即执行（不延迟一个周期） | 心跳类任务不应延迟首次上报 |
| 并发模型 | 每 Job 独立 goroutine | 隔离故障，单 Job panic 不影响其他 |
| 错误处理 | 默认 log.Error，可注入 ErrorHandler | 用户可对接告警/重试系统 |

### 使用方式

```go
import (
    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/job"
    "github.com/go-zeus/zeus/job/interval"
)

// 声明 Job Spec
heartbeat := job.Spec{
    Name:  "heartbeat",
    Every: 30 * time.Second,
    Handler: func(ctx context.Context) error {
        return reportHeartbeat(ctx)
    },
    Timeout: 5 * time.Second, // 单次执行最多 5s
}

// 自动装配：注册 + 启动 + 关闭全部自动化
app := components.NewApp(
    components.NewJobComponent(interval.New()),
    components.NewJobRegistration(heartbeat),
)
app.Run()
```

### 与 cluster 治理的协同

`Handler` 的 ctx 在 `Stop` 时被取消，业务可读取 ctx 内的 cluster 标记（如果通过 routing.WithCluster 注入）做集群差异化执行：

```go
job.Spec{
    Name:  "config-reload",
    Every: 1 * time.Minute,
    Handler: func(ctx context.Context) error {
        cluster := routing.FromContext(ctx) // 默认 default
        return reloadClusterConfig(cluster)
    },
}
```

### 完整示例

- `examples/job/`：interval 调度器 + 3 个 Job（不同间隔）+ ErrorHandler 告警钩子
- `examples/job-cron/`：cron 调度器 + URL scheme (`cron://`) + `@every` / cron 表达式

### URL scheme 切换调度器实现

通过 `job.NewSchedulerFromURL` 用 URL 字符串切换 interval / cron 实现：

```go
import (
    _ "github.com/go-zeus/zeus/job/interval"       // 注册 interval://
    _ "github.com/go-zeus/zeus/plugins/job/cron"   // 注册 cron://（需在 go.mod require 该插件）
)

s, _ := job.NewSchedulerFromURL("cron://?seconds=true&loc=UTC")
// s 装入 components.NewJobComponent(s) 即可
```

支持的 scheme：
- `interval://` → `interval.New()`（固定间隔）
- `cron://` → `cron.New()`（cron 表达式，支持 `seconds=true` / `loc=Asia/Shanghai` query 参数）

## 消息队列（mq 包）

`mq` 包提供发布/订阅（pub/sub）的统一抽象，参考 Dapr Building Block 设计，主包定义接口 + `Message`，内置 `memory` 实现作为进程内事件总线（也用作单测 mock），第三方实现（Kafka/NATS/Redis Streams）放在 `plugins/mq/<vendor>`。

### 核心概念

| 概念 | 说明 |
|------|------|
| `Message` | 消息体（Topic + Payload + Headers） |
| `Handler` | 消息处理函数（返回 nil=ack，error=nack） |
| `Publisher` | 发布者接口（Publish + Close） |
| `Subscriber` | 订阅者接口（Subscribe + Close） |
| `Broker` | 完整代理（同时实现 Publisher + Subscriber） |
| `memory.Broker` | 内置实现：channel fan-out，无缓冲反压，自动 baggage 注入/提取 |
| `MQComponent` | components 适配器：声明式注册 + 自动启停 |
| `MQSubscription` | 单个订阅包装为组件 |

### 设计权衡

| 维度 | 选择 | 理由 |
|------|------|------|
| 抽象层级 | 只抽象 topic / payload / headers | 屏蔽 Kafka partition / RabbitMQ exchange 等厂商专属语义 |
| ack 语义 | Handler 返回 error = nack | 不同实现可映射到不同动作（memory 走 ErrorHandler，Kafka 不 commit offset） |
| 内置实现 | 进程内 channel + 无缓冲 | 零依赖、保证不丢消息，反压慢消费者（牺牲吞吐换可靠） |
| 持久化 | 不持久化 | 用作单进程事件总线 / 测试 mock；生产用 plugins |
| Baggage 传播 | Publish 自动注入 msg.Headers["baggage"]，handler 自动 extract | 全链路 tenant.id / cluster 等 K-V 透传 |
| 并发模型 | 每订阅者独立 goroutine | fan-out 隔离故障 |

### 使用方式

```go
import (
    "github.com/go-zeus/zeus/components"
    "github.com/go-zeus/zeus/mq"
    "github.com/go-zeus/zeus/mq/memory"
)

// 1. 直接使用 Broker（无 components）
broker := memory.New()
defer broker.Close()

_ = broker.Subscribe(ctx, "orders.created", func(ctx context.Context, msg *mq.Message) error {
    // 处理订单
    return nil
})
_ = broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("order-1")})

// 2. 自动装配：注册 + 启动 + 关闭全部自动化
app := components.NewApp(
    components.NewMQComponent(memory.New()),
    components.NewMQSubscription("orders.created", handleOrder),
    components.NewMQSubscription("log.all", handleLog),
)
app.Run()

// 发布：业务代码持有 broker 引用（注册时传入），直接 Publish 即可
// ctx 内的 baggage 自动注入 msg.Headers["baggage"]
ctx = propagation.With(ctx, "tenant.id", "acme")
broker.Publish(ctx, "orders.created", &mq.Message{Payload: []byte("x")})
```

### 与 propagation 集成（baggage 自动传播）

| 位置 | 行为 |
|------|------|
| `Publish` 出口 | 自动 `InjectMetadata(ctx, msg.Headers)`：ctx baggage → `msg.Headers["baggage"]`（W3C 编码） |
| `handler` 入口 | 自动 `ExtractMetadata(ctx, msg.Headers)`：`msg.Headers["baggage"]` → handler ctx |
| `Handler` 内读取 | `propagation.Get(ctx, "tenant.id")` 直接拿到 |

订阅侧可通过 `MQComponent.OnStart` 时 `GetType[mq.Broker](ctx)` 获取 broker 实例。

### 不自动传播的场景

直接使用 Kafka/NATS 原生 SDK 绕过 zeus 抽象时，需手动注入/提取 baggage（参考 plugins/mq/kafka 实现模式）。

### 完整示例

参见 `examples/mq/`：演示 3 个订阅者（不同 topic）+ baggage 全链路传播 + 优雅关闭。

## 数据访问层（database + cache 包）

Zeus 提供数据库与缓存抽象，与现有 trace/metrics/propagation 体系无缝集成。设计原则：**薄封装 stdlib，不做 ORM**（用户可自由选 sqlx / gorm / ent / sqlc）。

### 数据库（database 包）

| 概念 | 说明 |
|------|------|
| `DB` / `Tx` / `Rows` / `Row` | 薄接口，签名几乎对齐 `*sql.DB`（学习成本零） |
| `DBOptions` | 连接池配置（Driver/DSN/MaxOpenConns/...） |
| `TxOption` | 事务选项（WithIsolation / WithReadOnly） |
| `WithTx`/`FromTx` | 同进程事务传播（多个 Repository 共享 Tx） |
| `WithTxID`/`TxIDFromContext`/`EnsureTxID` | 跨服务 tx_id 传播（审计/排查用，不做 2PC） |
| `database/sql` | 内置实现：薄封装 stdlib `*sql.DB` + 自动 trace/metrics/tx_id |
| `DatabaseComponent` | components 适配器：OnStart Ping，OnStop Close |

**自动集成矩阵**（每次 Query/Exec 触发）：

| 集成 | 行为 |
|------|------|
| **trace** | span `db.query`/`db.exec`/`db.tx.begin`/`db.tx.commit`/`db.tx.rollback`；attrs: `db`, `query`, `tx_id` |
| **metrics** | counter `db_query_total{db,op,status}` + histogram `db_query_duration{db,op}` |
| **propagation** | `EnsureTxID` 自动写入 `Bag{zeus.tx.id}`，随 client/server 跨服务透传 |

**使用方式**：

```go
import (
    "github.com/go-zeus/zeus/database"
    sqldriver "github.com/go-zeus/zeus/database/sql"
    _ "github.com/go-sql-driver/mysql"  // 注册驱动
)

db, _ := sqldriver.New(database.DBOptions{
    Driver: "mysql",
    DSN:    "user:pass@tcp(127.0.0.1:3306)/db",
    MaxOpenConns: 50,
}, tracer, meter)

// 单条操作
rows, _ := db.Query(ctx, "SELECT id, name FROM users WHERE age > ?", 18)

// 事务（同进程 tx 传播 + 自动 tx_id）
tx, _ := db.BeginTx(ctx)
ctx = database.WithTx(ctx, tx)
// repoA / repoB 内部 FromTx(ctx) 优先取事务句柄
_ = tx.Commit()

// 跨服务：tx_id 自动透传到下游（client 自动注入到 Baggage）
ctx = database.WithTxID(ctx, "biz-tx-001")
```

### 缓存（cache 包）

| 概念 | 说明 |
|------|------|
| `Cache` | 接口：Get/Set/Delete/Has/Close（与 Redis API 对齐） |
| `Item` | 载体：Key/Value/TTL |
| `Option` | `WithTTL(d)`（默认无 TTL = 永久） |
| `cache/memory` | 内置实现：sync.Map + TTL 双路径清理（懒 + 后台周期扫描） |
| `CacheComponent` | components 适配器：OnStop Close（停后台 goroutine） |

**自动集成矩阵**：

| 集成 | 行为 |
|------|------|
| **trace** | span `cache.get`/`cache.set`/`cache.delete`/`cache.has`；attrs: `cache`，可选 `cache_key`（默认关闭避免敏感数据） |
| **metrics** | counter `cache_op_total{cache,op,status}`（status: `hit`/`miss`/`ok`） + histogram `cache_op_duration{cache,op}` |

**使用方式**：

```go
import (
    "github.com/go-zeus/zeus/cache"
    "github.com/go-zeus/zeus/cache/memory"
)

c := memory.New(
    memory.WithTracer(tracer),
    memory.WithMeter(meter),
    memory.WithName("user-cache"),
    memory.WithCleanupInterval(time.Minute), // 默认 60s
)
defer c.Close()

_ = c.Set(ctx, "user:1", user, cache.WithTTL(5*time.Minute))
v, ok := c.Get(ctx, "user:1")  // (user, true) 或 (nil, false)
_ = c.Delete(ctx, "user:1")
```

### 设计权衡

| 维度 | 选择 | 理由 |
|------|------|------|
| database 抽象 | 薄封装 stdlib | 不与 sqlx/gorm/ent 竞争，保留原生 API |
| ORM 能力 | 不做 | 用户自治 |
| 跨服务事务 | 仅 tx_id 透传 | 不实现 2PC（业务侧选 Seata/TCC/Saga） |
| cache 后台清理 | 60s 默认 + 懒清理 | 兼顾内存与 CPU 开销 |
| cache key 记录 | 默认关闭 | 避免敏感数据进入 trace |

### 完整示例

- `examples/database/`：演示建表/插入/查询/事务/tx_id 透传（用 fake driver，无需真实 DB）
- `examples/cache/`：演示 Set/Get/Has/Delete/TTL 过期

### 接入真实驱动

内置 `database/sql`（薄封装 stdlib）+ `cache/memory` 已满足单进程零依赖场景。分布式 / 生产部署走 plugins：

```go
// plugins/database/mysql：薄包装 go-sql-driver/mysql + 复用主包全部 trace/metrics/tx_id
import (
    "github.com/go-zeus/zeus/database"
    mysql "github.com/go-zeus/zeus/plugins/database/mysql"
)
db, err := mysql.New(database.DBOptions{
    DSN:         mysql.BuildDSN("root", "pass", "127.0.0.1", mysql.DefaultPort, "test"),
    MaxOpenConns: 50,
}, tracer, meter)

// plugins/cache/redis：实现 cache.Cache，仅 string/[]byte
import (
    "github.com/redis/go-redis/v9"
    "github.com/go-zeus/zeus/cache"
    redis "github.com/go-zeus/zeus/plugins/cache/redis"
)
cli := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
c := redis.New(cli, redis.WithTracer(tracer), redis.WithMeter(meter))
defer c.Close() // 关闭 client（WithManagedClient(false) 可保留共享 client）

// 复杂类型需自行序列化
payload, _ := json.Marshal(user)
_ = c.Set(ctx, "user:1", payload, cache.WithTTL(5*time.Minute))
```
