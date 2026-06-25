// L4 示例：手动装配入口（app.New + WithServer）
//
// 推荐新用户从 examples/01-hello 开始（L1 5 行 API：app.Run(cfg, handler)）。
//
// 本例演示需要完全控制 server 装配的场景：
//
//	import (
//	    "github.com/go-zeus/zeus/app"
//	    "github.com/go-zeus/zeus/server/http"
//	)
//	    a := app.New(app.WithServer(http.NewHTTP(http.Port(8080))))
//	    a.Run(make(chan struct{}))
//
// 注意：旧的"副作用 import"模式（_ "github.com/go-zeus/zeus/server/http" + 不传 server）
// 已废弃。server.DefaultServer 全局变量已删除。
package main

import (
	"net/http"

	"github.com/go-zeus/zeus/app"
	zeushttp "github.com/go-zeus/zeus/server/http"
)

func main() {
	srv := zeushttp.NewHTTP(
		zeushttp.Port(8080),
		zeushttp.Mux(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello from zeus L4"))
		})),
	)
	a := app.New(app.WithServer(srv))
	_ = a.Run(make(chan struct{}))
}

// curl http://localhost:8080
// curl http://localhost:8080/health
