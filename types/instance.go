package types

import "github.com/go-zeus/zeus/metadata"

// Instance 服务实例
type Instance struct {
	Id       string              `json:"id"`
	Name     string              `json:"name"`    //服务名称
	Cluster  string              `json:"cluster"` //集群名称
	Ip       string              `json:"ip"`
	Port     int                 `json:"port"`
	Metadata metadata.MD         `json:"metadata"`
	Labels   map[string][]string `json:"labels"`
}
