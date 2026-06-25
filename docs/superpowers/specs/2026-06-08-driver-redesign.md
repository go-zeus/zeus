# Driver 架构现代化重构设计

## 日期
2026-06-08

## 问题

当前 Zeus 框架的 Driver 层存在三层不必要的间接：

1. **`driver/` 子包** — 接口放在子包，叫 `driver.Driver`，没有语义
2. **`init()` + `Register()` + `Open()`** — 全局可变状态 + 字符串耦合
3. **强制包装** — 简单域也套一层 Portable Type struct，1:1 委托无增值

## 设计原则

1. 接口定义在主包，用有意义的名字
2. 构造器注入，编译时安全
3. 只在需要时包装，简单域直接暴露接口

## 架构

```
之前（三层间接）：
  用户 → Portable Type (struct) → driver.Driver (interface) → 实现

之后（扁平）：
  简单域：用户 → 接口（即用户 API）                    → 实现
  复杂域：用户 → 包装类型 (struct，持有接口)            → 实现
```

## 12 个域的接口命名与设计

### 简单域（接口即用户 API，无需包装）

| 域 | 接口名 | 文件 | 实现 |
|---|---|---|---|
| encoding | `Codec` | encoding.go | json: `func New() encoding.Codec` |
| metrics | `Meter` | metrics.go | noop: `func New() metrics.Meter` |
| trace | `Tracer` | trace.go | noop: `func New() trace.Tracer` |

用户直接使用接口：
```go
c := json.New()  // 返回 encoding.Codec
data, _ := c.Marshal(obj)
```

### 中等域（接口即用户 API，实现提供选项）

| 域 | 接口名 | 文件 | 实现 |
|---|---|---|---|
| registry | `Registrar` / `Discovery` | registry.go | memory: `func New() registry.Registrar` |
| balancer | `Balancer` | balancer.go | random: `func New() balancer.Balancer` |
| server | `Server` | server.go | http: `func New(opts ...Option) server.Server` |
| middleware | `Interceptor` | middleware.go | recovery: `func New() middleware.Interceptor` |
| ratelimit | `Limiter` | ratelimit.go | token: `func New() ratelimit.Limiter` |
| retry | `Retrier` | retry.go | exponential: `func New() retry.Retrier` |

实现可通过选项定制：
```go
srv := http.New(server.Port(9090))  // 返回 server.Server
```

### 复杂域（需要包装层，接口不暴露给用户）

| 域 | 内部接口名 | 用户类型 | 包装增值 |
|---|---|---|---|
| config | `Loader` | `Config` struct | 缓存 + 并发安全 + Watch goroutine |
| log | `LogWriter` | `Logger` struct | With() + 包级便捷函数 + defaultLogger |
| circuitbreaker | `Breaker` | `CircuitBreaker` struct | Execute() 包装 Allow/MarkSuccess/MarkFailed |

用户使用包装类型，不感知内部接口：
```go
c, _ := config.New(file.New())
v := c.Get("key")
```

## 目录结构变更

### 之前
```
registry/
├── registry.go           # 用户 API (Registrar struct wrapping driver.Driver)
├── driver/
│   └── driver.go         # type Driver interface
└── memory/
    └── memory.go         # func init() { Register("memory", ...) }
```

### 之后
```
registry/
├── registry.go           # type Registrar interface + type Discovery interface
└── memory/
    └── memory.go         # func New() registry.Registrar
```

`driver/` 子包消失。接口在主包。

## 各域详细变更

### 1. encoding（简单域）

**encoding/encoding.go：**
```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    Name() string
}
```

**encoding/json/json.go：**
```go
func New() encoding.Codec { return &jsonCodec{} }
```

删除：`encoding/driver/`、`encoding/Codec` struct 包装、`init()`、`Register`、`Open`、`Drivers`

### 2. metrics（简单域）

**metrics/metrics.go：**
```go
type Meter interface {
    Counter(name string, labels map[string]string) Counter
    Histogram(name string, labels map[string]string) Histogram
    Gauge(name string, labels map[string]string) Gauge
    Close() error
}
type Counter interface { Inc() }
type Histogram interface { Observe(float64) }
type Gauge interface { Set(float64) }
```

**metrics/noop/noop.go：**
```go
func New() metrics.Meter { return &noopMeter{} }
```

删除：`metrics/driver/`、`metrics.Metrics` struct 包装、`init()`、`Register`、`Open`、`Drivers`

### 3. trace（简单域）

**trace/trace.go：**
```go
type Tracer interface {
    StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)
    Close() error
}
type Span interface {
    End()
    SetAttributes(map[string]string)
    SetName(string)
    RecordError(error)
    IsRecording() bool
}
```

**trace/noop/noop.go：**
```go
func New() trace.Tracer { return &noopTracer{} }
```

删除：`trace/driver/`、`trace.Tracer` struct 包装、`init()`、`Register`、`Open`

### 4. registry（中等域）

**registry/registry.go：**
```go
type Registrar interface {
    Register(ctx context.Context, ins *types.Instance) error
    Deregister(ctx context.Context, ins *types.Instance) error
}

type Discovery interface {
    Watch(ctx context.Context, serviceName string) (<-chan struct{}, error)
    GetService(ctx context.Context, serviceName string) (*types.Service, error)
}
```

**registry/memory/memory.go：**
```go
func New() registry.Registrar { return &memoryRegistry{} }
```

删除：`registry/driver/`、`Registrar/Discovery/Registry` struct 包装、`init()`、`Register`、`Open`、`Drivers`、`Driver()`

注：memory.New() 返回的实例同时实现 Registrar 和 Discovery。

### 5. balancer（中等域）

**balancer/balancer.go：**
```go
type Balancer interface {
    Next() (*types.Instance, error)
    Reload([]*types.Instance) Balancer
}
```

**balancer/random/random.go：**
```go
func New() balancer.Balancer { return &randomBalancer{} }
```

删除：`balancer/driver/`、`LoadBalancer` struct 包装、`init()`、`Register`、`Open`、`Drivers`、`Driver()`

### 6. server（中等域）

**server/server.go：**
```go
type Server interface {
    Protocol() string
    Endpoint() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

**server/http/http.go：**
```go
func New(opts ...Option) server.Server { ... }
```

删除：`server/driver/`、`serverWrapper` struct、`init()`、`Register`、`Open`、`Drivers`

### 7. middleware（中等域）

**middleware/middleware.go：**
```go
type Interceptor interface {
    Intercept(ctx context.Context, req Request, handler Handler) (Response, error)
    Name() string
}

type Chain []Interceptor

func (c Chain) Handle(ctx context.Context, req Request, handler Handler) (Response, error) { ... }
```

**middleware/recovery/recovery.go：**
```go
func New() middleware.Interceptor { return &recoveryInterceptor{} }
```

删除：`middleware/driver/`、`Middleware` struct 包装、`init()`、`Register`、`Open`、`Drivers`

### 8. ratelimit（中等域）

**ratelimit/ratelimit.go：**
```go
type Limiter interface {
    Allow() bool
    Reserve() WaitDuration
    Rate() float64
}
```

**ratelimit/token/token.go：**
```go
func New() ratelimit.Limiter { return &tokenLimiter{} }
```

删除：`ratelimit/driver/`、`RateLimiter` struct 包装、`init()`、`Register`、`Open`

### 9. retry（中等域）

**retry/retry.go：**
```go
type Retrier interface {
    Next() (time.Duration, bool)
    Reset()
    Count() int
}
```

**retry/exponential/exponential.go：**
```go
func New() retry.Retrier { return &expRetrier{} }
```

删除：`retry/driver/`、`Retry` struct 包装、`init()`、`Register`、`Open`、`Drivers`

修复：`Next()` 返回完整 `(time.Duration, bool)`，不再丢失 Duration

### 10. config（复杂域）

**config/config.go：**
```go
type Loader interface {
    Load() ([]KeyValue, error)
    Watch() (Watcher, error)
}

type Config struct {     // 用户类型，包装 Loader
    mu     sync.RWMutex
    loader Loader
    values map[string][]byte
    ...
}

func New(l Loader) (*Config, error) { ... }
func (c *Config) Get(key string) []byte { ... }
func (c *Config) Watch() error { ... }
func (c *Config) Close() error { ... }
```

**config/file/file.go：**
```go
func New() config.Loader { return &fileLoader{} }
```

删除：`config/driver/`、`init()`、`Register`、`Open`、`Drivers`

用户类型 `Config` 保留 — 它有增值（缓存、并发安全、Watch goroutine）

### 11. log（复杂域）

**log/log.go：**
```go
type LogWriter interface {
    Log(ctx context.Context, level Level, msg string, fields ...Field)
    Close() error
}

type Logger struct {    // 用户类型，包装 LogWriter
    writer LogWriter
    fields []Field
}

func New(w LogWriter) *Logger { return &Logger{writer: w} }
func (l *Logger) With(fields ...Field) *Logger { ... }
func (l *Logger) Info(msg string) { l.writer.Log(context.Background(), InfoLevel, msg, l.fields...) }
```

包级便捷函数保留（Debug/Info/Warn/Error + defaultLogger + SetDefault）。

**log/slog/slog.go：**
```go
func New() log.LogWriter { return &slogWriter{} }
```

删除：`log/driver/`、`init()`、`Register`、`Open`、`Drivers`、内置 stdDriver

### 12. circuitbreaker（复杂域）

**circuitbreaker/circuitbreaker.go：**
```go
type Breaker interface {
    Allow() error
    MarkSuccess()
    MarkFailed()
    State() State
}

type CircuitBreaker struct {  // 用户类型，包装 Breaker
    breaker Breaker
}

func New(b Breaker) *CircuitBreaker { return &CircuitBreaker{breaker: b} }
func (cb *CircuitBreaker) Execute(fn func() error) error { ... }
```

**circuitbreaker/count/count.go：**
```go
func New() circuitbreaker.Breaker { return &countBreaker{} }
```

删除：`circuitbreaker/driver/`、`init()`、`Register`、`Open`

## 删除清单

```
删除所有 driver/ 子包：
  registry/driver/       → 接口移入 registry.go
  balancer/driver/       → 接口移入 balancer.go
  server/driver/         → 接口移入 server.go
  log/driver/            → 接口移入 log.go
  config/driver/         → 接口移入 config.go
  encoding/driver/       → 接口移入 encoding.go
  middleware/driver/     → 接口移入 middleware.go
  circuitbreaker/driver/ → 接口移入 circuitbreaker.go
  ratelimit/driver/      → 接口移入 ratelimit.go
  retry/driver/          → 接口移入 retry.go
  metrics/driver/        → 接口移入 metrics.go
  trace/driver/          → 接口移入 trace.go

删除每个域用户 API 中的：
  init()、Register()、Open()、Drivers()

删除简单域的 Portable Type struct 包装：
  encoding.Codec struct     → 改为 interface
  metrics.Metrics struct    → 改为 interface（Meter）
  trace.Tracer struct       → 改为 interface
  middleware.Middleware struct → Chain + Interceptor
  retry.Retry struct        → 改为 interface（Retrier）
  ratelimit.RateLimiter struct → 改为 interface（Limiter）
  balancer.LoadBalancer struct → 改为 interface（Balancer）
```

## 新增域的标准模板

```
功能域/
├── 功能域.go           # interface 定义（简单域）/ interface + struct（复杂域）
└── 内置实现/
    └── 实现.go         # func New() 功能域.XxxInterface
```

最小模板（简单域）：
```go
// myfeature/myfeature.go
type MyFeature interface {
    DoSomething() error
}

// myfeature/builtin/builtin.go
func New() myfeature.MyFeature { return &impl{} }
```

## components 包适配

### 构造器改为注入 driver 实例

```go
// 之前（字符串耦合）
components.NewRegistryComponent("memory")
components.NewLogComponent("slog")

// 之后（编译时安全）
components.NewRegistryComponent(memory.New())
components.NewLogComponent(slog.New())
```

### 保留专用组件（有实质生命周期逻辑）

| 组件 | 保留原因 |
|------|---------|
| RegistryComponent | OnStart 注册 + OnStop 注销 |
| ServerComponent | OnStart 启动 + OnStop 优雅关闭 |
| ServiceComponent | 依赖 server + registry，OnStop 注销 |
| ConfigComponent | Provide 时加载配置 + 启动 Watch |

### 简化为 Adapt()（无实质生命周期逻辑）

以下组件只有 Provide 构造，Lifecycle 为空，用通用 `Adapt()` 替代：

```go
components.Adapt("log", slog.New())           // 替代 NewLogComponent
components.Adapt("trace", noop.New())         // 替代 NewTraceComponent
components.Adapt("metrics", noop.New())       // 替代 NewMetricsComponent
components.Adapt("circuitbreaker", counter.New())  // 替代 NewCircuitBreakerComponent
components.Adapt("ratelimit", token.New())    // 替代 NewRateLimitComponent
components.Adapt("retry", exponential.New())  // 替代 NewRetryComponent
components.Adapt("middleware", recovery.New()) // 替代 NewMiddlewareComponent
```

可删除的适配器文件（7 个）：log.go, trace.go, metrics.go, circuitbreaker.go, ratelimit.go, retry.go, middleware.go

### 最终用户组装

```go
app := components.NewApp(
    // 简单组件：Adapt 注入
    components.Adapt("log", slog.New()),
    components.Adapt("trace", noop.New()),
    components.Adapt("metrics", noop.New()),
    components.Adapt("circuitbreaker", counter.New()),

    // 复杂组件：专用适配器（有生命周期逻辑）
    components.NewConfigComponent(file.New()),
    components.NewRegistryComponent(memory.New()),
    components.NewServerComponent(http.New()),
    components.NewServiceComponent(),
)
app.Run()
```

## 验收标准

1. 12 个 `driver/` 子包全部删除
2. 所有 init()/Register/Open/Drivers 全部删除
3. 12 个内置实现全部导出 `New()` 返回主包接口
4. 简单域无包装 struct，直接暴露接口
5. 复杂域保留包装 struct（config/log/circuitbreaker）
6. `go vet` + `go build` + `go test -race ./...` 全部通过
7. 整体覆盖率 >= 75%
8. `circuitbreaker/count` 重命名为 `circuitbreaker/counter`
