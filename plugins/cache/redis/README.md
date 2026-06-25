# Redis 缓存插件

`cache.Cache` 接口的 Redis 实现，基于 `redis/go-redis/v9`，与内存缓存具备相同的 trace / metrics 自动集成。仅支持 `string` / `[]byte` 类型的 value，复杂类型需用户自行序列化后传入。

## 安装

主仓零依赖，使用本插件需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/cache/redis
```

插件是独立 module，不会污染主仓依赖。

## 使用

```go
package main

import (
    "context"
    "encoding/json"
    "time"

    "github.com/go-zeus/zeus/cache"
    "github.com/go-zeus/zeus/plugins/cache/redis"
    goredis "github.com/redis/go-redis/v9"
)

type User struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}

func main() {
    cli := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:6379"})

    c := redis.New(cli,
        redis.WithTracer(tracer),
        redis.WithMeter(meter),
        redis.WithName("user-cache"),
    )
    defer c.Close()

    ctx := context.Background()

    // string
    _ = c.Set(ctx, "greeting", "hello", cache.WithTTL(5*time.Minute))
    if v, ok := c.Get(ctx, "greeting"); ok {
        s, _ := v.(string) // 类型断言为 string
        _ = s
    }

    // 复杂类型：自行 marshal 成 []byte
    payload, _ := json.Marshal(&User{ID: 1, Name: "alice"})
    _ = c.Set(ctx, "user:1", payload, cache.WithTTL(time.Hour))
    if raw, ok := c.Get(ctx, "user:1"); ok {
        var u User
        _ = json.Unmarshal(raw.([]byte), &u)
    }

    // 其他操作
    _ = c.Delete(ctx, "user:1")
    exists := c.Has(ctx, "greeting")
    _ = exists
}
```

`Get` 命中时返回的 `any` 实际类型为 `string`（写入 `[]byte` 时也会被转回 string）。需要 `[]byte` 时自行 `[]byte(v.(string))` 转换。

## 选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `WithTracer(t)` | `trace.Tracer` | noop | 注入 Tracer，每次操作自动埋点 |
| `WithMeter(m)` | `metrics.Meter` | noop | 注入 Meter，上报 hit/miss/error 计数与延迟 |
| `WithName(name)` | `string` | `"redis"` | metric label 与 span attr 中的 cache 标识，多实例场景区分 |
| `WithRecordKey(b)` | `bool` | `false` | 是否在 span attrs 中记录 `cache_key`，默认关闭避免敏感数据进入 trace |
| `WithManagedClient(b)` | `bool` | `true` | `Close()` 时是否关闭底层 client；共享连接池时设为 `false` |

`redis.Client(c)` 可从 `cache.Cache` 取回底层 `*redis.Client`，用于 Pipeline / Pub-Sub / Lua 等高级操作。

## 依赖

- `github.com/redis/go-redis/v9`（由用户构造 `*redis.Client` 后注入）

## 集成

- 自动 trace span：`cache.get` / `cache.set` / `cache.delete` / `cache.has`，attrs 含 `cache`（可选 `cache_key`）
- 自动 metrics：counter `cache_op_total{cache,op,status}`（status: `hit` / `miss` / `ok` / `error`）+ histogram `cache_op_duration{cache,op}`
- 与 cache 主包接口 100% 兼容：可作为 `cache/memory` 的分布式替代无缝替换
- URL scheme：`import _ "plugins/cache/redis"` 后可用 `cache.NewFromURL("redis://127.0.0.1:6379/0?pool=50&name=user-cache")`（tracer / meter 需单独注入）
- 完整端到端示例参考 `examples/cache/`
