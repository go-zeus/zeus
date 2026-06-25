// Example: zeus 多协议反向代理网关
//
// 启动后监听 :8081，根据协议自动分流：
//   - HTTP/HTTPS：标准反向代理（注入 X-Forwarded-For/X-Real-IP/X-Request-ID）
//   - WebSocket：Hijack + raw io.Copy 双向透传
//   - SSE：禁用缓冲，事件流式透传
//
// gRPC 代理走独立 plugin 模块（plugins/proxy/grpc），需独立端口
//
// 测试命令：
//
//	curl http://localhost:8081/api           → HTTP 反向代理
//	curl -N -H "Accept: text/event-stream" http://localhost:8081/events → SSE 代理
//	wscat -c ws://localhost:8081/ws          → WebSocket 代理
package main

import (
	"net/http"
	"net/url"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/proxy"
)

func main() {
	// 静态后端模式：所有请求转发到 127.0.0.1:9000
	// 动态模式示例（需先在 memory 中注册实例）：
	//   dis := memory.New()
	//   dis.Register(ctx, &types.Instance{Name: "api", Cluster: "default", IP: "127.0.0.1", Port: 9000})
	//   p := proxy.New(proxy.WithSelector(proxy.NewDiscoverySelector("api", dis, roundrobin.New())))
	target, _ := url.Parse("http://127.0.0.1:9000")
	p := proxy.New(proxy.WithSelector(proxy.NewStaticSelector(target)))

	log.Info("proxy listening on :8081, backend=%s", target)
	log.Fatal(http.ListenAndServe(":8081", p))
}
