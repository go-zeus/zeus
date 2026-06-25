# 贡献指南

感谢你对 Zeus 的关注！本文档说明如何向 Zeus 贡献代码。

## 行为准则

参与本项目即代表你同意遵守 [Code of Conduct](./CODE_OF_CONDUCT.md)。请在所有交流中保持尊重与包容。

## 快速开始

### 环境要求

- Go 1.22+（主仓）/ Go 1.25+（完整 workspace，含 etcd/otel 插件）
- git 2.20+
- make（可选，用于批量操作）

### Fork 与克隆

```bash
git clone https://github.com/<your-username>/zeus.git
cd zeus
git remote add upstream https://github.com/go-zeus/zeus.git
```

### 本地开发

```bash
# 主仓构建与测试
go build ./...
go test -race -count=1 ./...

# 完整覆盖率
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1

# 插件测试（独立 module，需 GOWORK=off）
for mod in $(find plugins -name go.mod); do
  (cd "$(dirname "$mod")" && GOWORK=off go test ./...)
done
```

## 提交 PR 流程

### 1. 创建分支

从 `main` 拉取特性分支：

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature
```

分支命名约定：
- `feat/<scope>`：新功能（如 `feat/cache-redis-cluster`）
- `fix/<scope>`：bug 修复（如 `fix/proxy-websocket-leak`）
- `docs/<scope>`：文档（如 `docs/contributing-guide`）
- `refactor/<scope>`：重构（如 `refactor/roundrobin-atomic`）
- `test/<scope>`：测试补充（如 `test/database-sql-tx`）

### 2. 编写代码

遵循下文的代码规范。所有新代码必须有对应测试。

### 3. 提交 commit

使用规范化的 commit message：

```
<type>(<scope>): <subject>

<body>

<footer>
```

**type**：`feat` / `fix` / `docs` / `refactor` / `test` / `chore` / `perf` / `build` / `ci`

**scope**（可选）：受影响的包或模块，如 `cache` / `proxy` / `registry` / `plugins/mq/kafka`

**subject**：祈使句、现在时、首字母小写、不加句号、≤72 字符

示例：

```
feat(cache/redis): support cluster mode

Add redis cluster client support via WithClusterMode(true) option.
Cluster nodes discovered viaNewClient with RouteByLatency strategy.

Closes #42
```

### 4. 推送并创建 PR

```bash
git push origin feat/your-feature
```

在 GitHub 上创建 PR，目标分支 `main`。PR 描述需包含：

- **What**：做了什么
- **Why**：为什么做（关联 issue）
- **How**：如何实现（关键设计决策）
- **Test**：如何验证（测试命令 / 手动验证步骤）

### 5. 评审流程

- 维护者会在 3 个工作日内响应（参见 [SECURITY.md](./SECURITY.md) 的响应 SLA）
- 评审意见请直接在 PR 中回复，不要关闭重开
- 要求修改时，追加 commit 而非 force-push（便于评审追踪差异）
- 通过后由维护者 squash merge

## 代码规范

### Go 版本

- 主仓 `go.mod`：`go 1.22`（用户兼容性下限，不要擅自提升）
- 插件 Go 版本由直接依赖决定（参见 [site/content/reference/plugin-bom.md](./site/content/reference/plugin-bom.md)）
- `go.work`：取所有模块最大值

### 包结构

每个功能域遵循统一结构：

```
功能域/
├── 功能域.go          ← 接口定义 + 用户 API
├── 内置实现/           ← 零依赖，导出 New() 构造函数
└── (plugins/第三方实现) ← 有第三方依赖，独立 go.mod
```

### 命名约定

| 项 | 规则 | 示例 |
|---|---|---|
| 包名 | 全小写、单数、无下划线 | `roundrobin`（非 `round_robin`） |
| 接口名 | 名词，单方法接口加 `-er` | `Balancer` / `Reader` |
| 构造函数 | `New` 或 `NewXxx`，返回接口 | `func New() Balancer` |
| Options | `WithXxx` 函数式选项 | `WithTTL(5 * time.Second)` |
| 首字母缩写 | 全大写 | `URL` / `HTTP` / `ID` / `IP`（非 `Url` / `Http`） |
| 测试函数 | `Test<被测函数>_<场景>` | `TestAllow_ExceedBurst` |

### 注释规范

- **注释语言**：与现有代码保持一致（中文注释）
- **godoc 风格**：以标识符开头，动词开头，避免"用于 XXX"

```go
// Good
// Allow 返回是否允许请求通过。
// 桶空时返回 false，不阻塞调用方。

// Bad
// 用于判断是否允许
```

- 导出符号必须有 godoc（函数 / 类型 / 常量 / 变量）
- 包注释写在 `doc.go` 或 `<包名>.go` 顶部，以 `// Package xxx ...` 开头

### 错误处理

- 错误信息小写开头、无句号、包含上下文

```go
// Good
return fmt.Errorf("cache: get %q: %w", key, err)

// Bad
return fmt.Errorf("Failed to get key.")
```

- 业务错误用 `errors.New` 返回，包装底层错误用 `%w`
- 不要吞错误（`_ = err`），除非有明确说明

### 并发安全

- 共享状态用 `sync.Mutex` / `sync.RWMutex` / `atomic` 保护
- 避免在热路径持锁（参考 `balancer/roundrobin` 的 atomic 实现）
- 后台 goroutine 必须有退出机制（`ctx.Done()` 或 `stopCh`），不能泄漏
- 测试并发代码必须通过 `go test -race`

### 依赖管理

- **主仓零依赖**：所有第三方依赖只能放 `plugins/`
- **插件独立 module**：每个插件自己的 `go.mod` + `replace` 指向本地主仓
- **新增插件**：
  1. 在 `plugins/<domain>/<name>/` 创建目录
  2. `go mod init github.com/go-zeus/zeus/plugins/<domain>/<name>`
  3. 添加 `replace github.com/go-zeus/zeus => ../../..`
  4. 编写 README.md（参考现有插件）
  5. 注册 URL scheme（如适用，参考 `cache/resolver.go`）

## 测试要求

### 覆盖率

- 新增代码覆盖率 ≥ 85%
- 热路径必须有 benchmark（参考 `balancer/roundrobin/round_robin_bench_test.go`）
- 并发代码必须通过 `go test -race`

### 测试风格

```go
// Good：子测试 + 表驱动 + 清晰命名
func TestAllow_ExceedBurst(t *testing.T) {
    l := New(100, 2)
    l.Allow()
    l.Allow()
    if l.Allow() {
        t.Fatal("request exceeding burst should be rejected")
    }
}

// Bad：通用名 + 无场景说明
func TestAllow(t *testing.T) { ... }
```

### 测试工具

- 用 `t.TempDir()` 创建临时目录（自动清理）
- 用 `t.Helper()` 标记辅助函数（错误定位到调用方）
- 用 `t.Cleanup()` 注册清理逻辑
- mock 用手写 fake（不引入 mockgen 等代码生成工具）

## 设计原则

贡献代码前，请确认符合 Zeus 的核心设计哲学（详见 [CLAUDE.md - 设计目标](./CLAUDE.md#设计目标最高指导原则)）：

1. **内部复杂 + 外部简单**：用户 5 行代码启动，需要时能挖到底层
2. **4 层渐进暴露**：L1 `app.Run` / L2 Config / L3 `app.NewApp` / L4 `components.NewApp`
3. **不允许越层泄漏**：L1/L2 文档不能出现 Component / Container / Lifecycle 等内部概念
4. **默认装配不允许失败**：任何"必须配置才能跑"的字段都是设计缺陷
5. **零依赖主仓**：第三方依赖只能放 `plugins/`

新增功能前先回答："能不能在用户不需要时看不到？"

## API 兼容性

参见 [site/content/reference/api-stability.md](./site/content/reference/api-stability.md)：

- 🔒 **稳定 API**：禁止 breaking change（需走 deprecation 流程）
- 🧪 **实验 API**：可能调整，需在 godoc 标注 `// Experimental: ...`
- 🔬 **内部 API**：不保证兼容，不应被外部依赖

当前处于 v0.x 阶段，允许 breaking change，但必须在 CHANGELOG.md 中明确记录。

## 报告 Bug 与提建议

- Bug 报告：[GitHub Issues](https://github.com/go-zeus/zeus/issues)，使用 Bug 模板
- 安全漏洞：按 [SECURITY.md](./SECURITY.md) 流程私下披露，**不要**开公开 issue
- 功能建议：GitHub Discussions 或 Issue，说明场景与现有方案的不足

## 许可证

贡献的代码将在 [MIT License](./LICENSE) 下发布。提交 PR 即表示你同意该许可。
