本文档记录 Zeus 插件依赖版本治理策略，作为发版前对齐依据。主仓零依赖，BOM 仅约束 `plugins/*` 独立 module。

## 治理原则

### 1. 最小依赖原则

每个插件只声明其直接依赖，间接依赖由 `go mod tidy` 自动解析。禁止手动 pin 间接依赖版本（除非解决冲突）。

### 2. Go 版本对齐矩阵

Go 版本由**直接依赖的最低 Go 要求**决定，不强行统一：

| 插件 | Go 版本 | 原因 |
|---|---|---|
| `cache/redis` | 1.22 | go-redis/v9 无更高要求 |
| `database/mysql` | 1.22 | go-sql-driver/mysql |
| `database/postgres` | 1.22 | pgx/v5 |
| `database/sqlite` | 1.22 | modernc.org/sqlite |
| `mq/kafka` | 1.22 | sarama |
| `mq/nats` | 1.22 | nats.go |
| `registry/nacos` | 1.22 | nacos-sdk-go/v2 |
| `log/zap` | 1.22 | go.uber.org/zap |
| `log/file_rotate` | 1.22 | lumberjack.v2 |
| `job/cron` | 1.22 | robfig/cron/v3 |
| `middleware/metrics` | 1.22 | 无第三方直接依赖 |
| `middleware/tracing` | 1.22 | 无第三方直接依赖 |
| `encoding/protobuf` | 1.23 | google.golang.org/protobuf |
| `metrics/prometheus` | 1.23.0 (+toolchain go1.24.0) | client_golang 要求 |
| `client/grpc` | 1.25 | google.golang.org/grpc v1.81 |
| `server/grpc` | 1.25 | google.golang.org/grpc v1.81 |
| `proxy/grpc` | 1.25 | google.golang.org/grpc v1.81 |
| `registry/etcd` | 1.25 | etcd v3.6 强制要求 |
| `config/etcd` | 1.25 | etcd v3.6 强制要求 |
| `trace/otel` | 1.25 | otel v1.24+ |
| `config/k8s` | 1.26.0 | k8s.io/client-go v0.36.2 硬约束 |

**规则**：Go 版本由直接依赖的最低要求决定。`go mod tidy` 会自动还原为依赖要求的最低版本，不要手工降级。`config/k8s` 的 `go 1.26.0` 是 k8s client v0.36.2 的硬约束（k8s 项目跟随 Go 主线），本地需要 Go 1.26+ 工具链才能构建该插件。

### 3. 共享依赖对齐

跨多插件共享的直接依赖应保持版本一致：

| 依赖 | 当前版本 | 使用插件 |
|---|---|---|
| `go.uber.org/zap` | v1.27.0 / v1.28.0 | log/zap, registry/etcd, config/etcd |
| `go.uber.org/multierr` | v1.10.0 / v1.11.0 | log/zap, etcd 系列（间接） |
| `google.golang.org/grpc` | v1.81.1 | client/grpc, server/grpc, proxy/grpc |
| `github.com/cespare/xxhash/v2` | v2.2.0 / v2.3.0 | 多插件（间接） |
| `github.com/klauspost/compress` | v1.17.9 | mq/kafka, mq/nats |

**规则**：发版前用 `go work sync` 对齐间接依赖；直接依赖在 BOM 升级 PR 中统一更新。

### 4. 主仓引用策略

所有插件通过 `replace github.com/go-zeus/zeus => ../../..` 引用本地主仓。

- **开发期**：保留 replace，方便联调
- **发版时**：删除 replace，require 改为正式版本号（如 `v0.1.0`）

## 当前已知问题

### P2：共享依赖版本漂移

| 依赖 | 漂移版本 | 建议统一 |
|---|---|---|
| `go.uber.org/zap` | v1.27.0 (etcd 间接) / v1.28.0 (log/zap 直接) | v1.28.0（log/zap 已对齐，etcd 系列等其上游升级） |
| `go.uber.org/multierr` | v1.10.0 / v1.11.0 | v1.11.0（log/zap 已对齐） |
| `cespare/xxhash/v2` | v2.2.0 / v2.3.0 | v2.3.0（间接依赖，由 MVS 自动选最大值，无需手工对齐） |

**已完成的对齐**：`plugins/log/zap` 的 `go.uber.org/multierr` 从 v1.10.0 升到 v1.11.0（与 etcd 系列间接引入版本对齐）。

## 验证脚本

```bash
for f in plugins/*/go.mod plugins/*/*/go.mod; do
  echo "$f: $(grep '^go ' "$f")"
done | sort -t: -k2

grep -l "replace github.com/go-zeus/zeus" plugins/*/go.mod plugins/*/*/go.mod | wc -l
# 验证各插件可独立构建
for d in plugins/*/ plugins/*/*/; do
  if [ -f "$d/go.mod" ]; then
    (cd "$d" && GOWORK=off go build ./... 2>&1 | head -5)
  fi
done
```

## 升级流程

1. **BOM 升级 PR**：仅修改版本号，不改业务代码
2. **CI 验证**：所有插件 `GOWORK=off go test ./...` 必须通过
3. **发版检查**：发版前确认无 replace 残留，require 指向正式版本

## 与主仓的关系

- 主仓 `go.mod` 保持 `go 1.22`（用户兼容性下限）
- 插件 Go 版本可高于主仓（因插件是独立 module）
- `go.work` 取所有模块最大值（当前 1.25.0）
