# 14-cache · 缓存接入 + TTL

演示 `cache` 包的声明式接入：Set / Get / Has / Delete / TTL 过期。使用内存 cache（`cache/memory`），无需 Redis。

## 演示场景

- components 自动装配 CacheComponent（OnStop Close 后台清理 goroutine）
- 基本操作：Set / Get / Has / Delete
- TTL 过期验证（懒清理 + 后台周期扫描双路径）

## 启动与测试

```bash
cd examples/14-cache
go run .
```

预期输出：

```
[INFO] cache ready
[INFO] set key=greeting value=hello ttl=5s
[INFO] get key=greeting value=hello hit=true
[INFO] has key=greeting=true
[INFO] set key=temp value=today ttl=200ms
[INFO] get key=temp hit=false (expired)
[INFO] delete key=greeting
```

## 核心代码

```go
c := memory.New(memory.WithName("demo"), memory.WithCleanupInterval(time.Minute))
defer c.Close()

c.Set(ctx, "greeting", "hello", cache.WithTTL(5*time.Second))
v, ok := c.Get(ctx, "greeting")    // (hello, true)
c.Delete(ctx, "greeting")
```

每次操作自动注入 trace（span `cache.get` 等）+ metrics（`cache_op_total{op,status}`）。

## 接入 Redis

```go
import _ "github.com/go-zeus/zeus/plugins/cache/redis"
c, _ := cache.NewFromURL("redis://127.0.0.1:6379/0")
```

## 衔接

- 数据库接入 → [13-database](../13-database/)
- L2 URL scheme 切 cache → [04-config-driven](../04-config-driven/)
