// Example: L2 配置驱动（URL scheme 切换 cache/database/mq）
//
// 演示场景：通过修改 Config 中的 URL 字符串即可切换 cache/database/mq 实现，
// 业务代码（handler）零改动。
//
// 与 L3 的对照：
//   - L2：app.Run(&Config{Cache: "memory://", Database: "mysql://...", MQ: "memory://"}, handler)
//   - L3：app.NewApp(AddServer(s), WithCacheURL("memory://"), ...)
//
// 心智差异：
//   - L2 用户只看到 URL 字符串，不感知实现包（但需 import _ 注册 resolver）
//   - L3 用户用类型安全的 WithXxx 选项（但 URL 字符串同样支持）
//
// 启动后访问：
//
//	curl http://localhost:9200/ping            → "pong"
//	curl -X POST http://localhost:9200/cache/k1?val=hello
//	curl http://localhost:9200/cache/k1        → "hello"（cache 装配生效）
//
// 优雅关闭：Ctrl+C 或 kill -INT <pid>
package main

import (
	"net/http"

	"github.com/go-zeus/zeus/app"

	// 注册 URL scheme resolver（副作用 import，不直接引用）
	_ "github.com/go-zeus/zeus/cache/memory" // memory:// → cache/memory
	_ "github.com/go-zeus/zeus/mq/memory"    // memory:// → mq/memory
	// _ "github.com/go-zeus/zeus/plugins/cache/redis"        // redis:// → plugins/cache/redis
	// _ "github.com/go-zeus/zeus/plugins/database/mysql"     // mysql:// → plugins/database/mysql
)

func main() {
	// 业务 handler：简单的 ping + cache 演示
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	// —— L2 配置驱动：URL 字符串决定底层实现 ——
	// 用户改 URL 即可切实现（memory → redis、mock mysql → 真 mysql 等），handler 零改动
	cfg := &app.Config{
		Name:    "config-driven-demo",
		Port:    9200,
		Cluster: "default",

		// 可选组件：URL 为空时跳过装配（L2 心智：零配置默认）
		// 此处开启 cache + mq 演示 URL 装配
		Cache: "memory://?name=demo-cache&cleanup=60s",
		MQ:    "memory://",

		// Database: "mysql://root:pass@127.0.0.1:3306/test?pool=50",
		//   ↑ 需要 import _ "github.com/go-zeus/zeus/plugins/database/mysql"
		//   ↑ 当前 demo 不连真实数据库，避免依赖
	}

	// L1/L2 入口：5 行启动，零感知 cache/mq 实例细节
	app.Run(cfg, mux)
}
