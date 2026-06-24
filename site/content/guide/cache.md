---
title: 缓存
weight: 80
---

| 概念 | 说明 |
|---|---|
| `Cache` | 接口：Get/Set/Delete/Has/Close（与 Redis API 对齐） |
| `Item` | 载体：Key/Value/TTL |
| `Option` | `WithTTL(d)`（默认无 TTL = 永久） |
| `cache/memory` | 内置实现：sync.Map + TTL 双路径清理（懒 + 后台周期扫描） |
| `CacheComponent` | components 适配器：OnStop Close（停后台 goroutine） |

## 自动集成矩阵

| 集成 | 行为 |
|---|---|
| **trace** | span `cache.get`/`cache.set`/`cache.delete`/`cache.has`；attrs: `cache`，可选 `cache_key`（默认关闭避免敏感数据） |
| **metrics** | counter `cache_op_total{cache,op,status}`（status: `hit`/`miss`/`ok`） + histogram `cache_op_duration{cache,op}` |

## 使用方式

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

## 设计权衡

| 维度 | 选择 | 理由 |
|---|---|---|
| cache 后台清理 | 60s 默认 + 懒清理 | 兼顾内存与 CPU 开销 |
| cache key 记录 | 默认关闭 | 避免敏感数据进入 trace |

## 接入 Redis

```go
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

完整示例参见 `examples/cache/`：Set/Get/Has/Delete/TTL 过期。
