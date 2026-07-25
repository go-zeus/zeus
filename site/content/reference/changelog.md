本文件记录 Zeus 框架所有显著变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)，遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

> 项目处于 v0.x 阶段，API 仍可能发生破坏性变更。v1.0.0 起 API 冻结。

## [Unreleased]

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
