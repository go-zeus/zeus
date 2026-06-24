// Package nacos 提供基于 nacos-group/nacos-sdk-go/v2 的注册中心实现。
//
// 设计目的：
//   - 让 zeus 业务代码通过 nacos:// URL scheme 零感知接入阿里云 Nacos 注册中心
//   - 实现 registry.Registrar / registry.Discovery / registry.Watcher 三件套
//   - 与 etcd 形成国际+国内双栈注册中心覆盖
//
// 字段映射（zeus.Instance ↔ Nacos Instance）：
//
//	zeus.Instance.ID         → Nacos InstanceId（自动生成，本包用 IP:Port#Cluster 作为 key 做幂等）
//	zeus.Instance.Name       → Nacos ServiceName
//	zeus.Instance.IP/Port    → Nacos Ip/Port
//	zeus.Instance.Cluster    → Nacos Metadata["zeus.cluster"]
//	                        （不映射到 Nacos ClusterName，避免与 Nacos 物理集群概念混淆）
//	zeus.Instance.Protocol   → Nacos Metadata["zeus.protocol"]
//	zeus.Instance.Metadata   → Nacos Metadata（每个 K-V 一对一）
//
// 默认配置：
//   - 地址：127.0.0.1:8848
//   - Namespace：空（公共命名空间）
//   - Group：DEFAULT_GROUP
//   - Ephemeral：true（临时实例，SDK 心跳保活；进程崩溃后 30s 自动下线）
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/registry"
//	    nacosplugin "github.com/go-zeus/zeus/plugins/registry/nacos"
//	)
//
//	reg := nacosplugin.New(
//	    nacosplugin.WithServer("127.0.0.1", 8848),
//	    nacosplugin.WithNamespace("production"),
//	)
//	defer reg.Close()
//
//	_ = reg.Register(ctx, &types.Instance{
//	    Name: "my-app", IP: "10.0.0.1", Port: 8080,
//	})
//	entry, _ := reg.GetService(ctx, "my-app")
package nacos

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/types"
)

// 默认常量
const (
	// DefaultServerAddr 默认 Nacos server 地址（单机部署常用）
	DefaultServerAddr = "127.0.0.1"
	// DefaultServerPort 默认 Nacos server 端口
	DefaultServerPort = 8848
	// DefaultGroup 默认 Group（Nacos 习惯用法）
	DefaultGroup = "DEFAULT_GROUP"
	// DefaultClusterName 默认 Nacos ClusterName（Nacos 概念，与 zeus.Instance.Cluster 不同）
	DefaultClusterName = "DEFAULT"

	// metadata key 约定（用于 zeus 字段在 Nacos Metadata 中的命名空间）
	metaKeyCluster  = "zeus.cluster"
	metaKeyProtocol = "zeus.protocol"
)

// 编译期检查 nacosRegistry 实现了三个核心接口
var (
	_ registry.Registrar = (*nacosRegistry)(nil)
	_ registry.Discovery = (*nacosRegistry)(nil)
	_ registry.Watcher   = (*nacosRegistry)(nil)
)

// Option 函数式选项
type Option func(*nacosRegistry)

// WithServer 设置 Nacos server 地址（可多次调用追加多 server）
func WithServer(host string, port int) Option {
	return func(r *nacosRegistry) {
		if host != "" && port > 0 {
			r.servers = append(r.servers, constant.ServerConfig{
				IpAddr: host,
				Port:   uint64(port),
			})
		}
	}
}

// WithNamespace 设置 Nacos namespace ID（用于环境隔离，如 dev/staging/production）
func WithNamespace(ns string) Option {
	return func(r *nacosRegistry) {
		r.namespace = ns
	}
}

// WithGroup 设置 Group（默认 DEFAULT_GROUP）
func WithGroup(g string) Option {
	return func(r *nacosRegistry) {
		if g != "" {
			r.group = g
		}
	}

}

// WithCredentials 启用 Nacos 用户名/密码鉴权（Nacos 2.x 默认开启鉴权）
func WithCredentials(username, password string) Option {
	return func(r *nacosRegistry) {
		r.username = username
		r.password = password
	}
}

// WithAccessKey 启用阿里云 ACM/AWS 风格的 AccessKey/SecretKey 鉴权
func WithAccessKey(ak, sk string) Option {
	return func(r *nacosRegistry) {
		r.accessKey = ak
		r.secretKey = sk
	}
}

// WithNamingClient 注入外部 naming_client.INamingClient，跳过本包内部的拨号
//
// 用于：
//   - 单元测试时注入 mock
//   - 应用其他模块已建好 client，复用连接池
//
// 注入后 Close 不会关闭该 client（由外部所有者负责）
func WithNamingClient(c naming_client.INamingClient) Option {
	return func(r *nacosRegistry) {
		r.client = c
		r.ownsClient = false
	}
}

// WithClientParams 自定义底层 client 高级参数（心跳间隔、超时等）
//
// 用于覆盖 ClientConfig 字段（如 TimeoutMs/BeatInterval/LogDir）
func WithClientParams(fn func(*constant.ClientConfig)) Option {
	return func(r *nacosRegistry) {
		if fn != nil {
			r.clientConfigFn = fn
		}
	}
}

type nacosRegistry struct {
	// 配置（不可变）
	servers        []constant.ServerConfig
	namespace      string
	group          string
	username       string
	password       string
	accessKey      string
	secretKey      string
	clientConfigFn func(*constant.ClientConfig)

	// 运行时
	client     naming_client.INamingClient
	ownsClient bool

	// 订阅管理：每个 serviceName 一个 SubscribeContext（避免重复 Subscribe）
	mu          sync.Mutex
	subscribers map[string]*subscribeContext
}

type subscribeContext struct {
	cancel context.CancelFunc
	ch     chan struct{}
}

// New 创建 Nacos 注册中心
//
// 默认值：
//   - Server: 127.0.0.1:8848
//   - Group: DEFAULT_GROUP
//   - Namespace: ""（公共命名空间）
//
// 拨号是惰性的：New 只配置不连接，首次 Register/GetService 时才实际建立连接
func New(opts ...Option) registry.Registrar {
	r := &nacosRegistry{
		servers: []constant.ServerConfig{
			{
				IpAddr: DefaultServerAddr,
				Port:   uint64(DefaultServerPort),
			},
		},
		group:       DefaultGroup,
		subscribers: make(map[string]*subscribeContext),
		ownsClient:  true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// getClient 惰性建立 Nacos naming client
func (r *nacosRegistry) getClient() (naming_client.INamingClient, error) {
	if r.client != nil {
		return r.client, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}

	// ClientConfig
	cc := &constant.ClientConfig{
		NamespaceId:         r.namespace,
		NotLoadCacheAtStart: true,
		TimeoutMs:           10000,
		BeatInterval:        5000,
	}
	if r.username != "" || r.password != "" {
		cc.Username = r.username
		cc.Password = r.password
	}
	if r.accessKey != "" {
		cc.AccessKey = r.accessKey
		cc.SecretKey = r.secretKey
	}
	// 用户自定义覆盖默认
	if r.clientConfigFn != nil {
		r.clientConfigFn(cc)
	}

	client, err := clients.NewNamingClient(vo.NacosClientParam{
		ClientConfig:  cc,
		ServerConfigs: r.servers,
	})
	if err != nil {
		return nil, fmt.Errorf("nacos: create naming client (servers=%v, ns=%q): %w", r.servers, r.namespace, err)
	}
	r.client = client
	return client, nil
}

// toNacosInstance 把 zeus Instance 转换为 Nacos RegisterInstanceParam
func toNacosInstance(r *nacosRegistry, ins *types.Instance) vo.RegisterInstanceParam {
	metadata := make(map[string]string, len(ins.Metadata)+2)
	for k, v := range ins.Metadata {
		metadata[k] = v
	}
	// zeus 专属元数据
	if ins.Cluster != "" {
		metadata[metaKeyCluster] = ins.Cluster
	}
	if ins.Protocol != "" {
		metadata[metaKeyProtocol] = ins.Protocol
	}

	return vo.RegisterInstanceParam{
		Ip:          ins.IP,
		Port:        uint64(ins.Port),
		ServiceName: ins.Name,
		GroupName:   r.group,
		ClusterName: DefaultClusterName,
		Weight:      1.0,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true, // 心跳保活，进程崩溃后 30s 内 Nacos 自动下线
		Metadata:    metadata,
	}
}

// Register 注册实例到 Nacos
//
// 行为：
//   - 幂等：相同 Instance 多次 Register 会自动覆盖（Nacos 内部 dedup）
//   - Ephemeral=true：SDK 后台心跳保活，进程崩溃后 30s 自动下线
func (r *nacosRegistry) Register(ctx context.Context, ins *types.Instance) error {
	if ins == nil {
		return fmt.Errorf("nacos: register nil instance")
	}
	if ins.Name == "" || ins.IP == "" || ins.Port == 0 {
		return fmt.Errorf("nacos: instance Name/IP/Port are required")
	}

	client, err := r.getClient()
	if err != nil {
		return err
	}

	param := toNacosInstance(r, ins)
	ok, err := client.RegisterInstance(param)
	if err != nil {
		return fmt.Errorf("nacos: register %s/%s: %w", r.group, ins.Name, err)
	}
	if !ok {
		return fmt.Errorf("nacos: register %s/%s returned false", r.group, ins.Name)
	}
	return nil
}

// Deregister 反注册实例
func (r *nacosRegistry) Deregister(ctx context.Context, ins *types.Instance) error {
	if ins == nil || ins.Name == "" || ins.IP == "" || ins.Port == 0 {
		return nil // 幂等 no-op
	}

	client, err := r.getClient()
	if err != nil {
		return err
	}

	ok, err := client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          ins.IP,
		Port:        uint64(ins.Port),
		ServiceName: ins.Name,
		GroupName:   r.group,
		Cluster:     DefaultClusterName,
		Ephemeral:   true,
	})
	if err != nil {
		return fmt.Errorf("nacos: deregister %s/%s: %w", r.group, ins.Name, err)
	}
	if !ok {
		// ok=false 可能是实例本来就不存在，视为 no-op
		return nil
	}
	return nil
}

// GetService 拉取该 service 名下的全部健康实例并聚合为 *types.ServiceEntry
//
// 行为：
//   - 仅返回 Healthy=true 的实例（与 zeus 其他实现一致）
//   - 把 Nacos Metadata["zeus.cluster"] 还原回 zeus.Instance.Cluster
//   - 把 Nacos Metadata["zeus.protocol"] 还原回 zeus.Instance.Protocol
func (r *nacosRegistry) GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("nacos: empty service name")
	}

	client, err := r.getClient()
	if err != nil {
		return nil, err
	}

	instances, err := client.SelectInstances(vo.SelectInstancesParam{
		ServiceName: serviceName,
		GroupName:   r.group,
		HealthyOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("nacos: get service %q: %w", serviceName, err)
	}

	entry := types.NewServiceEntry()
	for _, ni := range instances {
		ins := nacosToZeus(ni, serviceName)
		_ = entry.AddInstance(ins)
	}

	if len(entry.Instances) == 0 {
		return nil, fmt.Errorf("nacos: service %q not found", serviceName)
	}
	return entry, nil
}

// nacosToZeus 把 Nacos instance 转换为 zeus Instance
func nacosToZeus(ni model.Instance, serviceName string) *types.Instance {
	ins := &types.Instance{
		ID:       getInstanceID(ni),
		Name:     serviceName,
		IP:       ni.Ip,
		Port:     int(ni.Port),
		Metadata: ni.Metadata,
	}
	if ni.Metadata != nil {
		if c, ok := ni.Metadata[metaKeyCluster]; ok {
			ins.Cluster = c
			delete(ins.Metadata, metaKeyCluster)
		}
		if p, ok := ni.Metadata[metaKeyProtocol]; ok {
			ins.Protocol = p
			delete(ins.Metadata, metaKeyProtocol)
		}
	}
	return ins
}

// getInstanceID 从 Nacos instance 提取业务 ID
func getInstanceID(ni model.Instance) string {
	if ni.InstanceId != "" {
		return ni.InstanceId
	}
	return ni.Ip + ":" + strconv.Itoa(int(ni.Port))
}

// Watch 监听服务名下的实例变更，每次变更推送 struct{}{} 到返回 channel
//
// 实现：用 Nacos Subscribe + callback，每次 callback 触发时把 struct{}{} 推送到 channel
// channel 容量 1：coalescing 语义，慢消费方不会阻塞 SDK 内部回调
//
// 注意：本方法只发"有变更"信号，订阅者需自行调用 GetService 拿最新列表
func (r *nacosRegistry) Watch(ctx context.Context, serviceName string) (<-chan struct{}, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("nacos: empty service name")
	}

	client, err := r.getClient()
	if err != nil {
		return nil, err
	}

	ch := make(chan struct{}, 1)

	// 立即推送一次，确保订阅者从最新状态起步
	ch <- struct{}{}

	subscribeCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	if old, exists := r.subscribers[serviceName]; exists {
		// 已有旧订阅：先取消
		old.cancel()
		close(old.ch)
	}
	r.subscribers[serviceName] = &subscribeContext{cancel: cancel, ch: ch}
	r.mu.Unlock()

	// Nacos callback：每次变更推送信号
	callback := func(services []model.Instance, err error) {
		if err != nil {
			return
		}
		select {
		case ch <- struct{}{}:
		default:
			// coalescing：丢弃未消费的通知
		}
	}

	subscribeParam := &vo.SubscribeParam{
		ServiceName:       serviceName,
		GroupName:         r.group,
		Clusters:          []string{DefaultClusterName},
		SubscribeCallback: callback,
	}
	err = client.Subscribe(subscribeParam)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("nacos: subscribe %q: %w", serviceName, err)
	}

	// 监听 ctx 取消 → Unsubscribe
	go func() {
		defer close(ch)
		<-subscribeCtx.Done()

		r.mu.Lock()
		delete(r.subscribers, serviceName)
		r.mu.Unlock()

		_ = client.Unsubscribe(subscribeParam)
	}()

	return ch, nil
}

// Close 关闭底层 Nacos client（仅在 ownsClient=true 时关闭）
// 可重复调用
func (r *nacosRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 取消所有订阅
	for name, sub := range r.subscribers {
		sub.cancel()
		delete(r.subscribers, name)
	}

	// Nacos SDK 的 client 没有显式 Close 方法（依赖 runtime.Goexit() 自然退出）
	// 这里仅清理引用，让 GC 回收
	r.client = nil
	return nil
}
