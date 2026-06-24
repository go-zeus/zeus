package main

import (
	"context"
	"io"
	"net/http"

	"github.com/go-zeus/zeus/balancer/random"
	"github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/types"
)

func main() {
	serviceName := "demo"

	// memory 同时实现 registry.Registrar 和 registry.Discovery
	mem := memory.New()

	// 注册实例
	mem.Register(context.Background(), &types.Instance{
		ID:      "1",
		Name:    serviceName,
		Cluster: "default",
		IP:      "127.0.0.1",
		Port:    8080,
	})

	// 类型断言获取 Discovery 接口
	dis := mem.(registry.Discovery)

	// 创建带服务发现和负载均衡的客户端
	demoClient := client.NewClient(serviceName,
		client.Discovery(dis),
		client.LoadBalance(random.New()),
	)

	r, _ := http.NewRequest("GET", "http://demo/", nil)
	res, err := demoClient.Do(r)
	if err != nil {
		log.Error("client do error:%v", err)
		return
	}
	data, err := io.ReadAll(res.Body)
	log.Info("%s err %v", data, err)
}
