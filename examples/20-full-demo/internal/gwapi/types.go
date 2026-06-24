// Package gwapi 定义 gateway 与 srv / 前端共享的 HTTP 协议类型。
//
// 设计说明：
//   - gateway 作为嵌入式服务注册中心，暴露 /internal/register 供 srv 自注册
//   - 暴露 /api/services 返回 JSON 实例列表，供 srv 的 HTTP discovery 拉取、前端可视化消费
//   - 类型与 zeus/types 解耦（JSON 友好），由各端按需转换
package gwapi

// Instance 实例的对外 JSON 表示
type Instance struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Cluster  string            `json:"cluster"`
	Protocol string            `json:"protocol"`
	IP       string            `json:"ip"`
	Port     int               `json:"port"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RegisterRequest 自注册请求体
type RegisterRequest struct {
	Instance Instance `json:"instance"`
}

// RegisterResponse 自注册响应体
type RegisterResponse struct {
	OK bool `json:"ok"`
}

// ServicesResponse 服务列表响应
type ServicesResponse struct {
	Services map[string][]Instance `json:"services"` // name → instances
}
