package components

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/metadata"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/server"
	"github.com/go-zeus/zeus/types"
	"github.com/go-zeus/zeus/utils/ip"
	"github.com/go-zeus/zeus/utils/uuid"
)

// 默认服务名与集群名
const (
	defaultServiceName = "zeus-service"
	defaultClusterName = "default"
)

// ServiceOption 配置 ServiceComponent 的选项
type ServiceOption func(*ServiceConfig)

// ServiceConfig 服务组件的配置（实例生成模板）
type ServiceConfig struct {
	Name    string
	Cluster string
	IP      string
}

// WithServiceName 设置服务名（注册中心的 key）
func WithServiceName(name string) ServiceOption {
	return func(c *ServiceConfig) {
		c.Name = name
	}
}

// WithServiceCluster 设置集群名（路由 key）
func WithServiceCluster(cluster string) ServiceOption {
	return func(c *ServiceConfig) {
		c.Cluster = cluster
	}
}

// WithServiceIP 设置实例 IP（默认自动探测本机 IP）
func WithServiceIP(ip string) ServiceOption {
	return func(c *ServiceConfig) {
		c.IP = ip
	}
}

// ServiceComponent 服务组件适配器
//
// 依赖：
//   - server（必填）：从中获取 []server.Server
//   - registry（可选）：未配置则跳过注册
//
// 生命周期：
//   - Provide：为每个 server 生成一个 Instance（带 Protocol 字段）
//   - OnStart：若配置了 registry，遍历实例调用 Registrar.Register
//   - OnStop：若配置了 registry，遍历实例调用 Registrar.Deregister
type ServiceComponent struct {
	opts ServiceConfig

	// instances 每个 server 对应一个 Instance（多 server 场景下长度 > 1）
	instances []*types.Instance
}

// NewServiceComponent 创建服务组件
//
// opts 用于覆盖默认值：WithServiceName(...) / WithServiceCluster(...) / WithServiceIP(...)
func NewServiceComponent(opts ...ServiceOption) *ServiceComponent {
	cfg := ServiceConfig{
		Name:    defaultServiceName,
		Cluster: defaultClusterName,
		IP:      ip.LocalIP(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &ServiceComponent{opts: cfg}
}

func (sc *ServiceComponent) Name() string { return "service" }
func (sc *ServiceComponent) Depends() []string {
	return []string{"server"}
}

func (sc *ServiceComponent) Provide(ctx Context) (any, error) {
	// 从容器获取 []server.Server（按类型，无需手动断言）
	servers, err := Type[[]server.Server](ctx)
	if err != nil {
		return nil, fmt.Errorf("service: server dependency missing: %w", err)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("service: no server available")
	}

	// 为每个 server 生成一个 Instance（带 Protocol）
	sc.instances = make([]*types.Instance, 0, len(servers))
	for _, srv := range servers {
		host, port := parseEndpoint(srv.Endpoint())
		// server Endpoint 可能不含 IP（":8080"），用配置的 IP 兜底
		if host == "" {
			host = sc.opts.IP
		}
		ins := &types.Instance{
			ID:       uuid.New(),
			Name:     sc.opts.Name,
			Cluster:  sc.opts.Cluster,
			Protocol: srv.Protocol(),
			IP:       host,
			Port:     port,
			Metadata: make(metadata.MD),
			Labels:   make(map[string][]string),
		}
		sc.instances = append(sc.instances, ins)
	}
	return sc.instances, nil
}

func (sc *ServiceComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			if len(sc.instances) == 0 {
				return nil
			}
			// registry 可选：未配置则仅启动 server，不注册
			r, err := Type[registry.Registrar](ctx)
			if err != nil {
				log.Info("service %s starting (no registry, skip register)", sc.opts.Name)
				return nil
			}
			// 遍历所有 instance 注册（沿用容器注入的 ctx，感知外部取消/超时）
			// 30s 兼容真实注册中心（etcd/consul）首次拨号 + TLS 协商
			regCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			for _, ins := range sc.instances {
				if err := r.Register(regCtx, ins); err != nil {
					return fmt.Errorf("register instance %s (%s): %w", ins.ID, ins.Protocol, err)
				}
				log.Info("service %s registered: %s:%d (protocol=%s, cluster=%s)",
					ins.Name, ins.IP, ins.Port, ins.Protocol, ins.Cluster)
			}
			return nil
		},
		OnStop: func(ctx Context) error {
			if len(sc.instances) == 0 {
				return nil
			}
			log.Info("service %s stopping, deregistering %d instance(s)...", sc.opts.Name, len(sc.instances))
			// 反注册（与 OnStart 对称：无 registry 则跳过）
			r, err := Type[registry.Registrar](ctx)
			if err != nil {
				return nil
			}
			deregCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			for _, ins := range sc.instances {
				if err := r.Deregister(deregCtx, ins); err != nil {
					// 反注册失败仅记录日志，不阻塞关闭流程
					log.Error("deregister instance %s (%s): %v", ins.ID, ins.Protocol, err)
				} else {
					log.Info("service %s deregistered: %s", ins.Name, ins.ID)
				}
			}
			return nil
		},
	}
}

// parseEndpoint 解析 server.Endpoint() 返回的 "host:port" 字符串
// 返回 (host, port)；若解析失败，port 返回 0
//
// 使用 net.SplitHostPort 支持 IPv6 形如 "[::1]:8080"，比手写 strings.LastIndex 更安全。
// 输入若不含端口（如 "127.0.0.1"），降级为 host-only 解析，port=0。
func parseEndpoint(endpoint string) (host string, port int) {
	h, p, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint, 0
	}
	port, _ = strconv.Atoi(p)
	return h, port
}
