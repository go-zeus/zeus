// Example: cache
//
// 演示声明式缓存接入：
//   - 使用内存 cache（无需 Redis）
//   - 通过 components 自动装配：注册 → 启动 → 优雅关闭 全部自动化
//   - 演示：Set / Get / Has / Delete / TTL 过期
//
// 启动：
//
//	go run .
//
// 预期输出：
//
//	[INFO] cache demo starting；Ctrl+C to stop
//	[INFO] cache ready
//	[INFO] set key=greeting value=hello ttl=5s
//	[INFO] get key=greeting value=hello hit=true
//	[INFO] has key=greeting=true
//	[INFO] set key=temp value=today ttl=200ms
//	[INFO] get key=temp hit=false (expired)
//	[INFO] delete key=greeting
//	[INFO] cache demo done；Ctrl+C 退出
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/cache/memory"
	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"
)

func main() {
	c := memory.New(
		memory.WithName("demo-cache"),
		memory.WithCleanupInterval(0), // 演示用懒清理即可
	)

	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewCacheComponent(c),
	)

	log.Info("cache demo starting；Ctrl+C to stop")

	go runDemo(c)

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}

func runDemo(c cache.Cache) {
	time.Sleep(100 * time.Millisecond) // 等组件启动

	ctx := context.Background()

	// 1) Set + Get（带 TTL）
	_ = c.Set(ctx, "greeting", "hello", cache.WithTTL(5*time.Second))
	log.Info("set key=greeting value=hello ttl=5s")

	if v, ok := c.Get(ctx, "greeting"); ok {
		log.Info("get key=greeting value=%s hit=true", v)
	} else {
		log.Info("get key=greeting hit=false (unexpected)")
	}

	// 2) Has
	log.Info("has key=greeting=%t", c.Has(ctx, "greeting"))

	// 3) TTL 过期
	_ = c.Set(ctx, "temp", "today", cache.WithTTL(200*time.Millisecond))
	log.Info("set key=temp value=today ttl=200ms")
	time.Sleep(300 * time.Millisecond)
	if _, ok := c.Get(ctx, "temp"); !ok {
		log.Info("get key=temp hit=false (expired)")
	}

	// 4) Delete
	_ = c.Delete(ctx, "greeting")
	log.Info("delete key=greeting")
	if _, ok := c.Get(ctx, "greeting"); !ok {
		log.Info("get key=greeting after delete hit=false (ok)")
	}

	// 5) 各种 value 类型
	_ = c.Set(ctx, "num", 42)
	_ = c.Set(ctx, "user", struct{ Name string }{"alice"})
	if v, _ := c.Get(ctx, "num"); v != 42 {
		log.Info("get key=num value=%v", v)
	}
	if v, _ := c.Get(ctx, "user"); fmt.Sprint(v) != "{alice}" {
		log.Info("get key=user value=%v", v)
	}

	log.Info("cache demo done；Ctrl+C 退出")
}
