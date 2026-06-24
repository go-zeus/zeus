# Changelog

本文件记录 Zeus 框架所有显著变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)，遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

> 项目处于 v0.x 阶段，API 仍可能发生破坏性变更。v1.0.0 起 API 冻结。

## [Unreleased]

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
