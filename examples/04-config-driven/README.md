# 04-config-driven · L2 配置驱动（URL scheme 切换 cache/mq）

修改 `app.Config` 中的 URL 字符串即可切换 cache / database / mq 实现，业务 handler 零改动。演示 Zeus "改配置不改代码"的 L2 心智。

## 核心能力

| Config 字段 | URL 示例 | 实现 |
|---|---|---|
| `Cache` | `memory://?cleanup=60s` | cache/memory |
| `Database` | `mysql://user:pass@host/db` | plugins/database/mysql |
| `MQ` | `memory://` | mq/memory |

切换实现只需改 URL + 副作用 import 对应 plugin（`import _ ".../plugins/cache/redis"` 等）。

## 启动与测试

```bash
cd examples/04-config-driven
go run .
```

```bash
curl http://localhost:9200/ping
# pong

# 写入 cache（Config.Cache="memory://" 已装配 cache 组件）
curl -X POST "http://localhost:9200/cache/k1?val=hello"
# stored

# 读取 cache（验证 cache 装配生效）
curl http://localhost:9200/cache/k1
# hello
```

## 核心代码

```go
import (
    _ "github.com/go-zeus/zeus/cache/memory" // 注册 memory:// scheme
    _ "github.com/go-zeus/zeus/mq/memory"    // 注册 memory:// (mq)
)

app.Run(&app.Config{
    Port:   9200,
    Cache:  "memory://",   // 改这一行即可切 redis://
    MQ:     "memory://",   // 改这一行即可切 kafka://
}, handler)
```

## 衔接

- L1 零配置启动 → [01-hello](../01-hello/)
- L3 类型装配（`WithCacheURL` Option）→ [03-typed](../03-typed/)
- cache 完整用法（TTL/Has/Delete）→ [14-cache](../14-cache/)
