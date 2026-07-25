本文件记录 Zeus 框架所有显著变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)，遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

> 项目处于 v0.x 阶段，API 仍可能发生破坏性变更。v1.0.0 起 API 冻结。

## [Unreleased]

### Fixed — 深度审计：并发安全 / 资源管理 / 错误处理加固

- **`batch`**：`Close` 用 `sync.Once` 保证幂等，修并发/重复 Close 的 double-close panic；`callHandler` 的 handler panic 不再静默吞掉（默认标准库 log 输出 + 新增 `WithErrorHandler` Option 对接告警）
- **`utils/async`**：`Exec` 的 ctx 取消分支返回 zero value，修与后台 goroutine 写 `result` 的 data race（与 `ExecCtx` 已修范式对齐）
- **`registry/memory`**：`Close` 实现 `io.Closer`（返回 error），使 `components.RegistryComponent.OnStop` 能通过类型断言自动关闭默认 memory registry（原签名不匹配导致 watcher channel 不被关闭，下游阻塞监听者无法感知退出）
- **`mq/memory`**：`Close` 加 10s 默认超时，避免不响应 ctx 的 handler 导致 `Close` 永久阻塞、应用无法优雅关闭（SIGKILL 后丢失 in-flight 消息）
- **`types.ServiceEntry`**：`Reload` 跳过重复 ID，修 `Instances` 覆盖写入但 `Cluster.AddInstance` 拒绝导致的两个索引不一致（路由层与状态层行为分裂）
- **`config`**：`Watch` 的 watcher goroutine 加 panic recover（对齐全包 goroutine 入口约定，防 plugin 实现 panic 拖垮进程）
- **`utils/ctxutil`**：`DoneOrBlock` 的 goroutine 加 recover，避免 fn panic 时调用方永久阻塞（无 ctx 取消场景下 select 永等）
- **`client`**：`watcher` goroutine 加 recover（防 plugin registry 实现 panic 拖垮进程）
- **`server/http`**（安全）：`TLSFiles` 证书加载失败改为 fail-fast（`Start` 返回 error），拒绝静默降级为明文 HTTP（原行为造成 HTTPS/mTLS 预期实际暴露为 HTTP）
- **`utils/uuid`**：`New` 在 `crypto/rand` 失败时 fallback 到时间戳+原子计数器 ID（避免返回空串导致 `Instance.ID` 大面积冲突）
- **`middleware/requestid`**：`generateID` 在 `crypto/rand` 失败时 fallback（避免全零 ID 导致 trace 关联失效）
- **`database/sql`**：删除 `QueryRow`/`tx.QueryRow` 中多余的 `_ = ctx` 死代码（ctx 已用于 `startSpan`，baggage 经 spanCtx 正确传递）

### Fixed — 深度审计：核心运行时生命周期与类型装配（阶段B）

- **`components/container`**：`stopReverse` 去掉 `ctx.Err` 早退，best-effort 调用所有 OnStop（原超时跳过剩余组件，导致 trace flush / DB 连接池关闭 / cache 清理被静默漏关，造成 span 丢失、连接泄漏、goroutine 累积）；结尾仍返回 `ctx.Err` 通知"关闭不完整"
- **`components/context`**：`assemblyContext.mu` 改 `*sync.RWMutex` 指针，`withContext` 派生 ctx 共享同一把锁（原值字段导致派生 ctx 拿独立新锁，并发读写 `providers`/`byType` 是数据竞争，race detector 触发）
- **`components/server`**：`OnStop` 的 `wg.Wait` 加 ctx 保护，避免某些 Server 实现的 Serve 不响应 Shutdown 时永久阻塞、卡死整个关闭流程
- **`app/quickstart` + `CLAUDE.md`**：默认中间件链顺序修正为 `recovery → requestid → accesslog`（代码实际语义 recovery 最外层捕获所有 panic；原文档 `requestid → accesslog → recovery` 错误）
- **`components/resolve`**：循环依赖报错列出环上候选节点，便于定位
- **`components/database`**：`OnStart` Ping 用容器注入的 ctx 派生超时（原 `context.Background()` 不响应启动期取消）
- **`app/options`**：L3 装配（`WithMeter`/`WithTracer`）检测同名 L4 组件并跳过默认，避免重复 `Register` panic（修复 L3/L4 混用卖点在 metrics/trace 维度的崩溃）
- **`app/app`**：L4 手动模式用 `errors.Join` 聚合 run 与 stop 错误（原 stop 错误被 run 错误覆盖丢失，关闭期资源泄漏信号被吞）

### Fixed — 深度审计：数据/治理域算法与并发（阶段C）

负载均衡与缓存：
- **`cache/memory`**：后台清理 `cleanupExpired` 改用 `CompareAndDelete`（CAS），与 `Get` 懒清理一致，修并发 `Set` 写入新值后被误删的 ABA；`Close` 等待 cleaner goroutine 退出，避免 `Close` 返回后仍有残留写
- **`balancer/roundrobin`**：`Reload` 随机起始游标，避免服务发现频繁推流后首轮 `Next` 系统性偏向 `instances[0]`；`Reload` 浅拷贝实例 slice 防外部别名
- **`balancer/random`**：改用 `math/rand/v2`（lock-free PCG）替代全局 `math/rand` 互斥锁；`Reload` 浅拷贝

消息队列：
- **`mq/memory`**：`Publish` 每订阅者深拷贝 `*Message`（Headers map 独立），修多 handler 共享同一 msg 的 data race；不再修改调用方 msg（Topic/Headers 回填改在本地副本）；并发 `Close` 导致全部订阅者已退出时返回 error（原静默返回 nil 让调用方误以为发布成功）

治理算法（核心 bug）：
- **`ratelimit/token`**：`NewWithOptions` 初始令牌统一满桶（`tokens=burst`），修原 `tokens=rate` 导致 `rate<burst` 时初始不满桶、首请求被错误拒绝
- **`ratelimit/token`**：`Reserve` 预占令牌（tokens 减为负值），修原不扣减导致 N 个并发 `Reserve` 各自等待后全部放行的超卖（限流彻底失效）
- **`circuitbreaker/counter`**：`Allow` 的 Open→HalfOpen 转换本次即计入探测（`halfOpenCnt=1`），修原 off-by-one 导致 `halfOpenMax=N` 实际放行 N+1（破坏"单请求试探"语义）；未知状态 fail-closed（拒绝）；`State()` 去除虚拟转换副作用（观察与实际放行一致）
- **`retry/exponential`**：`Next` 在 float 域 clamp + `IsInf`/`IsNaN` 防护，修大 `count` 时 `math.Pow` 返回 `+Inf` 经饱和转换为负 `Duration`、退避归零引发重试风暴

数据库：
- **`database/sql`**：`BeginTx` 空 TxOption 传 `nil`（与 stdlib"未指定"语义一致）；`QueryRow` 文档明确 metrics status 恒 ok（错误延迟到 Scan，需准确错误 metrics 请用 `Query`）

### Fixed — 深度审计：可观测链路一致性（阶段D）

- **`propagation/baggage`**（Critical）：`isTokenChar` 移除 `%`，修编解码不对称——原 `%` 在白名单导致 Encode 不转义，Decode 的 `PathUnescape` 遇孤立 `%` 报 "invalid URL escape" 丢弃整个 entry（如 value `100%` 跨进程后丢失，且无任何错误提示）。现 `%` 总被 encode（`100%` → `100%25`），编解码对称
- **`middleware/accesslog`**：改用 `log.Default().Log(r.Context(), ...)`，让注入的 logger（`WithLogger`/`SetDefault`）生效且 cluster/baggage 经 ctx 自动注入为 Field（原包级 `log.Info` 用 `context.Background`，access log 永远缺 cluster 标记，违反三件套联动一致性）
- **`metrics/noop`**：`Counter`/`Histogram`/`Gauge` 改返回共享单例，避免每次操作堆分配（落实"noop 零开销"承诺）
- **`log/slog`**：`Log` 透传 ctx（原用 `context.Background`，slog handler 无法基于 ctx 做 sampling/trace 关联）
- **`utils/uuid`**：`fallbackUUID` 去掉 v4 标记位（应急 ID 非 RFC 4122 v4 随机，诚实标识不假冒 v4）

### Changed (Breaking) — utils 工具包重设计 + 运行时加固

- **`utils/time` → `utils/timex`**：包重命名并收敛 API。新增布局常量（`DateTime`/`DateTimeMs`/`DateOnly`/`TimeOnly`）+ 可变参数默认布局的 `Format`/`Parse` + 范围辅助（`BeginningOfDay`/`EndOfDay`/`BeginningOfWeek` 等）。import 路径与包名同步变更
- **`utils/set`**：统一为指针接收者。构造函数 `New`/`NewWithCapacity`/`FromSlice` 返回 `*Set[T]`，`SortedValues` 签名改为接收 `*Set[T]`。修复「值接收者 + 可变 map」导致 `s2 := s1; s2.Add(x)` 隐式修改 `s1` 的 footgun；现 `s2 := s1` 为显式共享语义，独立副本须用 `Clone()`
- **`utils/event`**：`OneEvent`/`OnceEvent` 标记 Deprecated（别名保留）。`Event` 合并原 `OneEvent` 语义（支持多次触发，默认 `keepLatest` 合并）；新增 `Latch` 取代 `OnceEvent`

**迁移方法**：
- `utils/time` → 搜索替换 import 路径为 `utils/timex`，包名 `time` → `timex`
- `set` → 调用点改指针语义（编译器会逐处报错引导，零静默破坏）
- `event` → `OneEvent` 用 `Event` 代替、`OnceEvent` 用 `Latch` 代替（旧名仍可用，见下 Deprecated）

### Deprecated

- **`event.OneEvent` / `event.NewOneEvent`**：用 `Event` / `NewEvent` 代替
- **`event.OnceEvent` / `event.NewOnceEvent`**：用 `Latch` / `NewLatch` 代替
- 保留周期见 [api-stability.md](./api-stability.md) 的 Deprecation Policy（v1.0.0 前不删除）

### Added — utils 工具包增强

- **`event.Latch` / `NewLatch`**：一次性事件（`Trigger() bool` / `Done() <-chan struct{}` / `HasFired() bool`），供多等待方观察同一完成信号
- **`event.WithKeepOldest()`**：`Event` 合并策略选项（连触多次只保留首个，默认 `keepLatest`）
- **`random.FastRange` / `FastInt` / `FastString`**：基于 `math/rand/v2` 的快速伪随机路径（~5ns，非密码学安全），与 `crypto/rand` 安全路径双轨
- **`log.Fatalf` / `Logger.Fatalf`**：`Fatal` 的格式化版本
- **`banner`**：TTY 门控（非终端不输出）+ `ZEUS_NO_BANNER` 环境变量 + ldflags 版本注入
- **`set`**：完整集合代数（指针语义）`Clone` / `Union` / `Intersect` / `Difference` / `SymmetricDifference` / `IsSubset` / `IsSuperset` / `IsDisjoint` / `Equal`
- **cluster 治理**（`circuitbreaker/cluster` / `ratelimit/cluster` / `retry/cluster`）：`RemoveKey` / `Delete`，防动态 cluster key 内存泄漏

### Fixed — 运行时核心 Bug 修复

- **`snowflake`**：时钟回拨睡眠后未重新校验，导致生成重复 ID（睡眠后补重检守卫）
- **`registry/memory`**：`GetService` 返回内部指针，并发 map 迭代/写入 panic；改为返回 `ServiceEntry.Snapshot()` 快照
- **`mq/memory`**：`Close` 死锁、幽灵订阅者、handler panic 拖垮整个 broker（延迟恢复 + 退出清理 + done 分支）
- **`proxy`**：默认 transport 改用连接池（替换 `http.DefaultTransport`）；SSE 注入转发头并过滤逐跳头（RFC 7230 §6.1）；WebSocket 修复丢帧（用 `clientBuf` 而非 `clientConn`）
- **`server/http`**：默认 `ReadHeaderTimeout=10s` / `IdleTimeout=120s`（Slowloris 防御）
- **`cache/memory`**：`Get` 用 `CompareAndDelete` 防 ABA
- **`circuitbreaker`**：`Execute` panic 时正确 `MarkFailed` 再 re-panic（避免状态不一致）

### Changed (Breaking) — 命名规范对齐 Go 官方风格

以下变更属于 Go 命名规范对齐（参考 Effective Go + CodeReviewComments）：

- **`types.Instance`**：`Id` → **`ID`**、`Ip` → **`IP`**（首字母缩写全大写）
- **`components`**：`GetType[T]` → **`Type[T]`**、`GetAllByType[T]` → **`AllByType[T]`**（Go 惯例：导出函数不加 `Get` 前缀，参考 `http.Get`/`os.Stat`）
- **`balancer/round_robin`** → **`balancer/roundrobin`**（包名禁用下划线，参考 [Go Package Style Guide](https://go.dev/doc/effective_go#package-names)）
- **`utils/url.GetURL`** → **`URL`**（同上，移除 Get 前缀）
- **`utils/url.URL`** 返回值：`Url string` → **`URL string`**
- **`server/http.Ip(ip string)`** → **`IP(ip string)`**（首字母缩写）
- **`components.ServiceConfig.Ip`** → **`IP`**

**迁移方法**：搜索替换 + `go build ./...` 全仓扫描修复引用点。受影响范围：
- `types.Instance.{Id,Ip}`：8 处内部引用 + examples/20-full-demo 的 gwapi types
- `components.{GetType,GetAllByType}`：15+ 处内部引用（components/* 内部）
- `balancer/round_robin`：9 处 import 站点
- 其他：仅暴露 API，无内部影响

### Added — API 增强

- **`errors.Error.As(target any) bool`**：实现 `errors.As` 协议，支持业务侧用 `errors.As(err, &target)` 把业务错误解包到自定义结构体。对齐 kratos/errors 行为
- **`cache/memory.cacheImpl.done`**：内部 channel，cleaner goroutine 退出时关闭。替代 `runtime.NumGoroutine()` 数值比较，提升测试稳定性
- **`app/doc.go`**：独立的包文档文件，包含 L1-L4 分层 API 说明 + 文件职责

### Fixed — Bug 修复

- **`cache/memory`**：`WithCleanupInterval(0)` 实际不生效（原 `if d > 0` 守卫阻断了 0 的赋值，导致禁用 cleaner 时 cleaner 仍然启动）。修复后语义与文档一致：`d <= 0` 真正禁用后台清理
- **`cache/memory.TestClose_StopsCleaner`**：flaky 测试修复。原判定依赖 `runtime.NumGoroutine()` 数值（会被其他并行测试的 cleaner goroutine 干扰），改为 channel-based 退出信号判定（done channel 关闭即 goroutine 退出）
- **`app/options_test.TestNewApp_MixedWithL3`**：数据竞争修复。`mockCacheForL3.closeCalled` 原为 `sync.Mutex+bool`（写入有锁、读取无锁），改为 `atomic.Bool` 单字段同步
- **`examples/20-full-demo/internal/gwdisc/discovery_test.go`**：预先存在的并发 map 读写竞争修复。mock gateway handler 访问 `current.Services` map 加 `sync.Mutex` 保护

### Documentation — 文档规范对齐

- **`types/service.go`**：`NewServiceEntry/AddInstance/DelInstance/Reload/AllClusterName/AllCluster` 等方法补全 godoc
- **`types/cluster.go`**：`NewCluster/AddInstance/DelInstance/DelInstanceAndCount/GetInstances` 等方法补全 godoc
- **`testutil/testutil.go`**：`WaitUntil` godoc 完善（参数：timeout/interval/cond；返回值；行为：首次立即执行，超时补充检查）
- **`validation/validation.go`**：`Rule[T]` 泛型函数 godoc 完善（参数语义 + 链式返回值）
- **`database/database.go`**：`ErrNoTx` godoc 完善（触发场景 + 处理建议：errors.Is 检查，不要 panic）
- **注释风格统一**：多处 `// 用于 XXX` 反模式 → `// 典型场景：XXX` 或动词开头（参考 godoc 规范）
  - 涉及：`middleware/clustering`、`page`、`proxy/selector`、`server/http/cluster`、`utils/uuid`、`components/context`、`batch`、`mq` 等

### Examples — 工程化重构

- **目录编号化**：22 个 examples 目录按 L1→L4 学习路径加编号前缀（`00-app-quickstart`、`01-hello`、`02-with-registry`、`03-typed`、... `20-full-demo`、`21-registry-etcd`）
- **go.work 独立化**：每个 example 独立 go.mod（参考 go-zero/kitex 工程化标准），用户 `cp -r` 即用，依赖隔离避免重依赖 example 拉低整体构建
- **二进制清理 + .gitignore 加固**：清理约 30 个无后缀编译产物，新增 .gitignore 规则忽略 `examples/*/bin/` 和 `examples/*/build/` 下编译产物（保留 ca-certificates.crt 等构建输入）
- **examples/20-full-demo**：目录改名 `examples/full-demo` → `examples/20-full-demo`，同步更新内部 import 8 处

### Removed

- **`app/options.go` 顶部包注释**：迁移到独立文件 `app/doc.go`（godoc 包级注释规范）
