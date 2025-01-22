package main

import (
    "github.com/go-zeus/zeus/balancer/random"
    "github.com/go-zeus/zeus/client"
    "github.com/go-zeus/zeus/log"
    "github.com/go-zeus/zeus/registry/memory"
    "github.com/go-zeus/zeus/types"
    "io"
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
    demoClient := client.NewClient(serviceName, client.Discovery(dis), client.LoadBalance(random.NewRandom()))
    r, _ := http.NewRequest("GET", "http://demo/", nil)
    res, err := demoClient.Do(r)
    if err != nil {
        log.Error("client do error:%v", err)
        return
    }
    data, err := io.ReadAll(res.Body)
    log.Info("%s err %v", data, err)
}
