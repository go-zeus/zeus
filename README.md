# zeus
## 介绍
zeus是一个0依赖可插拔的微服务框架，基于微服务架构的抽象，其实现在zeus-plugin项目中，开发者可根据自己的技术栈进行灵活配置
## 开始
```go
package main

import (
	"github.com/go-zeus/zeus/app"
	"github.com/go-zeus/zeus/log"
)

func main() {
	log.Fatal(app.New().Run(make(chan struct{})))
}

// curl http://localhost:8080
```
## 路由
```go
package main

import (
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/server"
	"github.com/go-zeus/zeus/service"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/hi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test server"))
	})
	ser := server.New(server.Mux(mux))
	log.Fatal(service.New(service.Server(ser)).Run(make(<-chan struct{})))
}

// curl http://localhost:8080/hi
```
## 示例项目
https://github.com/go-zeus/zeus/tree/main/examples
