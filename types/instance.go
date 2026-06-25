package types

import "github.com/go-zeus/zeus/metadata"

// Instance 服务实例（一个进程的一个监听端口对应一个 Instance）
//
// 多协议应用（如 HTTP+gRPC 同进程）会注册多条 Instance，
// 通过 Protocol 字段区分（对齐 K8s Endpoints / Istio ServiceEntry 的多端口模型）。
type Instance struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`     //服务名称
	Cluster  string              `json:"cluster"`  //集群名称
	Protocol string              `json:"protocol"` //协议：http / grpc / ...
	IP       string              `json:"ip"`
	Port     int                 `json:"port"`
	Metadata metadata.MD         `json:"metadata"`
	Labels   map[string][]string `json:"labels"`
}
