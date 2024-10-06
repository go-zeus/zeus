package main

import (
	"github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/types"
	"net/http"
)

func main() {
	serviceName := "demo"
	dis := memory.NewMemory()
	dis.Register(&types.Instance{
		Id:      "1",
		Name:    serviceName,
		Cluster: "default",
		Ip:      "127.0.0.1",
		Port:    8080,
	})
	demoClient := client.NewClient(serviceName, client.Discovery(dis))
	r, _ := http.NewRequest("GET", "http://demo/", nil)
	res, err := demoClient.Do(r)
	body := make([]byte, res.ContentLength)
	_, _ = res.Body.Read(body)
	log.Info("%s err %v", body, err)
}
