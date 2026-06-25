本文档定义 Zeus 框架在 `v0.x` 阶段各 API 表面的稳定性等级，帮助用户判断依赖风险。

> v1.0.0 起，全部"稳定"级 API 进入 SemVer 兼容承诺（破坏性变更必须升 major 版本）。

## 稳定性等级定义

| 等级 | 含义 | 用户策略 |
|------|------|----------|
| 🔒 **稳定（Stable）** | v0.x 内承诺不破坏，仅在严重设计缺陷时才会改（会提供迁移期） | 可放心用于生产 |
| 🧪 **实验（Experimental）** | 当前迭代可能调整，但会给出迁移说明 | 评估后再用，关注 changelog |
| 🔬 **内部（Internal）** | 实现细节，无承诺 | 不要直接依赖，可能随时改名/删除 |

## L1 API 表面（最稳定）

| 符号 | 等级 | 说明 |
|------|------|------|
| `app.Run(ctx, cfg, handler)` | 🔒 稳定 | 入口函数签名冻结 |
| `app.Config` 结构体字段 | 🔒 稳定 | 已有字段不删除/不改语义；新字段可追加 |
| `app.DefaultConfig()` | 🔒 稳定 | 返回零值可用配置 |

**冻结理由**：L1 是面向"5 行启动"的入口，破坏会让所有用户重写。任何调整必须升 major。

## L2 API 表面（稳定）

| 符号 | 等级 | 说明 |
|------|------|------|
| URL scheme 协议：`memory://` / `etcd://` / `k8s://` / `redis://` / `mysql://` / `nats://` / `cron://` | 🔒 稳定 | 已注册 scheme 不改语义；query 参数可追加 |
| `Config.Registry/Cache/Database/MQ/Job` URL 字段 | 🔒 稳定 | 已有字段不删除 |
| Handler 类型推断规则（`http.Handler` → HTTP，`*grpc.Server` → gRPC） | 🔒 稳定 | 推断行为不变 |

## L3 API 表面（稳定）

| 符号 | 等级 | 说明 |
|------|------|------|
| `app.NewApp(opts ...any) *components.App` | 🔒 稳定 | 函数签名 + 返回类型 |
| `app.AddServer(s)` / `app.WithLogger(l)` / `app.WithRegistry(r)` / `app.WithMeter(m)` / `app.WithTracer(t)` / `app.WithMiddleware(mw)` / `app.WithServiceName(n)` / `app.WithServiceCluster(c)` / `app.WithServiceIP(ip)` / `app.WithStopTimeout(d)` / `app.WithComponent(c)` / `app.WithCacheURL(url)` / `app.WithDatabaseURL(url)` / `app.WithMQURL(url)` | 🔒 稳定 | Option 名称和签名 |
| `*components.App` 的 `Run()/Stop()` 方法 | 🔒 稳定 | 生命周期入口 |

## L4 API 表面（稳定 + 实验混合）

| 符号 | 等级 | 说明 |
|------|------|------|
| `components.NewApp(comps...) *App` | 🔒 稳定 | 主入口 |
| `components.Container` | 🔒 稳定 | 容器接口 |
| `Component` 接口（`Name/Depends/Provide/Lifecycle`） | 🔒 稳定 | 组件协议 |
| `Lifecycle` 接口（`OnStart/OnStop`） | 🔒 稳定 | 生命周期钩子 |
| `components.Type[T]` / `components.AllByType[T]` | 🔒 稳定 | 泛型装配函数（v0.x 已重命名一次：GetType→Type） |
| `components.Context` 接口 | 🔒 稳定 | 装配上下文 |
| 所有 `components.NewXxxComponent(...)` 适配器 | 🧪 实验 | 名称/参数列表可能调整 |
| `components.Adapt(name, instance)` | 🧪 实验 | 快速包装，签名可能微调 |

## 数据模型（稳定）

| 符号 | 等级 | 说明 |
|------|------|------|
| `types.Instance` 字段：`ID/Name/Cluster/Protocol/IP/Port/Metadata/Labels` | 🔒 稳定 | 字段名 + 类型 + JSON tag |
| `types.Cluster` | 🔒 稳定 | 同上 |
| `types.ServiceEntry` | 🔒 稳定 | 同上；`type Service = ServiceEntry` alias 同样稳定 |
| `metadata.MD` 类型 | 🔒 稳定 | 与 grpc/metadata 兼容 |

## 功能域接口（稳定）

以下接口的 **方法签名 + 行为契约** 承诺稳定：

| 功能域 | 接口 | 等级 |
|--------|------|------|
| registry | `Registrar` / `Discovery` / `Watcher` / `Instance` | 🔒 稳定 |
| server | `Server` 接口 | 🔒 稳定 |
| balancer | `Balancer` 接口 | 🔒 稳定 |
| middleware | `Interceptor` / `Chain` / `Request` / `Response` | 🔒 稳定 |
| log | `Writer` / `Field` / `Logger` | 🔒 稳定 |
| config | `Loader` / `Watcher` / `Decoder` | 🔒 稳定 |
| encoding | `Codec` 接口 | 🔒 稳定 |
| circuitbreaker | `Breaker` / `cluster.ClusterBreaker` | 🔒 稳定 |
| ratelimit | `Limiter` / `cluster.ClusterLimiter` | 🔒 稳定 |
| retry | `Retrier` / `cluster.ClusterRetrier` | 🔒 稳定 |
| metrics | `Meter` / `Counter` / `Histogram` / `Gauge` | 🔒 稳定 |
| trace | `Tracer` / `Span` / `SpanConfig` / `SpanOption` | 🔒 稳定 |
| propagation | `Bag` / `Entry` / `With` / `Get` / `InjectHTTP` / `ExtractHTTP` / `InjectMetadata*` / `ExtractMetadata*` | 🔒 稳定 |
| routing | `WithCluster` / `FromContext` / `ClusterFromHTTPHeader` / `HeaderCluster` / `MetadataCluster` / `Default` | 🔒 稳定 |
| proxy | `Proxy` / `Selector` / `NewStaticSelector` / `NewDiscoverySelector` | 🔒 稳定 |
| job | `Scheduler` / `Spec` | 🔒 稳定 |
| mq | `Publisher` / `Subscriber` / `Broker` / `Message` / `Handler` | 🔒 稳定 |
| database | `DB` / `Tx` / `Rows` / `Row` / `DBOptions` / `TxOption` / `WithTx` / `FromTx` / `WithTxID` / `TxIDFromContext` / `EnsureTxID` | 🔒 稳定 |
| cache | `Cache` / `Item` / `Option` / `WithTTL` | 🔒 稳定 |
| client | `HTTPClient` / `Client`（别名）/ `NewClient` | 🔒 稳定 |
| batch | `Batcher[T]` / `New` / `Add` / `TryAdd` / `AddContext` / `Flush` / `Close` | 🔒 稳定 |
| errors | `Error` 结构体 / `New` / `Error.As()` / `Error.Is()` / `Error.Unwrap()` | 🔒 稳定 |

## 协议契约（稳定）

| 协议 | 等级 | 说明 |
|------|------|------|
| HTTP Header `X-Zeus-Cluster` | 🔒 稳定 | 跨进程 cluster 标识 |
| gRPC metadata key `x-zeus-cluster` | 🔒 稳定 | 同上 |
| W3C Baggage 兼容的 `Baggage` HTTP Header | 🔒 稳定 | 用户自定义 K-V 全链路传播 |
| Baggage key 命名空间 `zeus.*` | 🔒 稳定 | 框架保留前缀（如 `zeus.cluster`、`zeus.tx.id`） |
| Metrics 命名空间 `zeus_*` | 🔒 稳定 | 框架保留前缀（如 `zeus_requests_total`、`zeus_request_duration_seconds`） |
| Trace span 命名 `{domain}.{op}`（如 `cache.get`、`db.query`） | 🔒 稳定 | span name 规范 |
| Trace attribute `zeus.cluster` / `zeus.tx.id` | 🔒 稳定 | 框架保留 attribute key |

## 不冻结的内容（仍可能调整）

以下内容在 v0.x 阶段不承诺稳定，仍可能调整：

- 各 Option 的**默认值**（例如 cleanupInterval 默认 60s 可能改为 30s）
- 各实现的**错误返回格式**（错误消息文本可能调整，但错误类型/Is 谓词稳定）
- **测试辅助包**（`testutil` / 内部 mock）的导出符号
- **辅助工具包**（`batch`、`safe`、`utils/*`）的导出函数
- 各 plugin 内部的**实现细节**（私有结构体字段、私有函数）

## 变更原则

1. **稳定 API 的破坏**：仅在严重设计缺陷时才会考虑，必须：
   - 在 CHANGELOG 中详细记录
   - 提供至少一个 minor 版本的迁移期（标记旧 API 为 `// Deprecated:`）
   - 给出迁移示例

2. **新增 API 不算破坏**：即使加在新版本也兼容旧版本。

3. **行为变更（不改签名）**：如果改变行为会影响用户，必须在 CHANGELOG 明确标注，并在 release notes 中突出说明。

## 当前 v0.x 阶段的破坏性变更窗口

`v0.1.0-alpha.1` ~ `v0.9.x` 期间，"实验"和"内部"级 API 可能在任何 minor 版本破坏。

`v1.0.0` 起所有"稳定"级 API 进入正式 SemVer 兼容承诺。

## 反馈

如果你正在评估 Zeus 用于生产，但某个"实验"级 API 对你很关键，请在 [GitHub Discussions](https://github.com/go-zeus/zeus/discussions) 提出，我们会评估是否升级为"稳定"。
