// Package etcd 提供基于 go.etcd.io/etcd/client/v3 的注册中心实现。
//
// 设计要点：
//   - 注册单位是 Instance，key 形如 `<prefix>/<service>/<id>`，按 service 前缀可拉全量
//   - 使用 lease + KeepAlive：进程崩溃后 TTL 到期自动反注册，无需依赖主动 Deregister
//   - Watch 通过 etcd Watcher 推送变更事件，触发订阅者重新 GetService
//   - GetService 返回 *types.ServiceEntry，二级索引（Instances/Clusters）由 types 包聚合
//
// 默认 etcd 地址：127.0.0.1:2379
// 修改方式：etcd.New(etcd.WithEndpoints("10.0.0.1:2379"))
package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/types"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	// DefaultEndpoint 用户指定的默认 etcd 地址
	DefaultEndpoint = "127.0.0.1:2379"

	// DefaultTTL lease 默认有效期。KeepAlive 每 TTL/3 续约一次，进程退出后 TTL 内自动过期。
	// 不能太短（网络抖动会误判下线），不能太长（崩溃后下线延迟）。
	DefaultTTL = 30 * time.Second

	// DefaultDialTimeout 默认拨号超时。
	// etcd client v3 内部是 gRPC，首次连接包含 TCP + TLS + HTTP/2 协商，
	// 5s 经常不够（特别是跨网段或慢速 DNS）。取 30s 兼顾网络抖动与启动速度。
	DefaultDialTimeout = 30 * time.Second

	// DefaultPrefix etcd key 前缀，按 service 名做层级划分。
	// 选 /zeus/services/ 而非 /zeus/，留出 /zeus/config /zeus/cluster 等扩展空间。
	DefaultPrefix = "/zeus/services/"
)

// 编译期检查 etcdRegistry 实现了三个核心接口
var (
	_ registry.Registrar = (*etcdRegistry)(nil)
	_ registry.Discovery = (*etcdRegistry)(nil)
	_ registry.Watcher   = (*etcdRegistry)(nil)
)

// Option 函数式选项
type Option func(*etcdRegistry)

// WithEndpoints 设置一个或多个 etcd 集群节点地址（host:port 形式）。
// 不调用本选项时使用 DefaultEndpoint。
func WithEndpoints(endpoints ...string) Option {
	return func(r *etcdRegistry) {
		if len(endpoints) > 0 {
			r.endpoints = append([]string(nil), endpoints...)
		}
	}
}

// WithTTL 设置 lease 有效期，必须 > 5s（etcd KeepAlive 最小间隔限制）。
// 进程退出后该时长内 etcd 自动删除 key，达到自动反注册效果。
func WithTTL(ttl time.Duration) Option {
	return func(r *etcdRegistry) {
		if ttl >= 5*time.Second {
			r.ttl = ttl
		}
	}
}

// WithPrefix 自定义 etcd key 前缀，便于多套环境隔离（如 dev/staging/prod）。
func WithPrefix(prefix string) Option {
	return func(r *etcdRegistry) {
		if prefix != "" {
			r.prefix = prefix
		}
	}
}

// WithCredentials 启用 etcd 用户名/密码鉴权。
func WithCredentials(username, password string) Option {
	return func(r *etcdRegistry) {
		r.username = username
		r.password = password
	}
}

// WithClient 注入外部 *clientv3.Client，跳过本包内部的拨号。
// 用于：
//   - 单元测试时注入 mock client
//   - 应用其他模块已建好 client，复用连接池
//
// 注入后 Close 不会关闭该 client（由外部所有者负责）。
func WithClient(cli *clientv3.Client) Option {
	return func(r *etcdRegistry) {
		r.client = cli
		r.ownsClient = false
	}
}

// WithDialTimeout 设置首次连接 etcd 的拨号超时。
func WithDialTimeout(d time.Duration) Option {
	return func(r *etcdRegistry) {
		if d > 0 {
			r.dialTimeout = d
		}
	}
}

type etcdRegistry struct {
	// 配置（不可变，New 后只读）
	endpoints   []string
	ttl         time.Duration
	prefix      string
	username    string
	password    string
	dialTimeout time.Duration

	// 运行时
	client     *clientv3.Client
	ownsClient bool // 是否拥有 client 所有权（决定 Close 时是否关闭）

	mu     sync.Mutex
	leases map[string]*leaseEntry // key: instance Id → lease 续约上下文
}

type leaseEntry struct {
	leaseID clientv3.LeaseID
	cancel  context.CancelFunc // 用于停止 KeepAlive goroutine
}

// New 创建 etcd 注册中心。
//
// 默认值：
//   - Endpoints: [DefaultEndpoint]（127.0.0.1:2379）
//   - TTL: 30s
//   - Prefix: /zeus/services/
//
// 拨号是惰性的：New 只配置不连接，首次 Register/GetService 时才实际建立连接。
// 这避免在测试/启动阶段因 etcd 不可达直接 panic。
func New(opts ...Option) registry.Registrar {
	r := &etcdRegistry{
		endpoints:   []string{DefaultEndpoint},
		ttl:         DefaultTTL,
		prefix:      DefaultPrefix,
		dialTimeout: DefaultDialTimeout,
		leases:      make(map[string]*leaseEntry),
		ownsClient:  true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// getClient 惰性建立 client（线程安全）
func (r *etcdRegistry) getClient() (*clientv3.Client, error) {
	if r.client != nil {
		return r.client, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	cfg := clientv3.Config{
		Endpoints:   r.endpoints,
		DialTimeout: r.dialTimeout,
	}
	if r.username != "" || r.password != "" {
		cfg.Username = r.username
		cfg.Password = r.password
	}
	cli, err := clientv3.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("etcd: dial %v: %w", r.endpoints, err)
	}
	r.client = cli
	return cli, nil
}

// instanceKey 由 Instance 字段拼接 key：prefix/<name>/<id>
func (r *etcdRegistry) instanceKey(ins *types.Instance) string {
	return path.Join(r.prefix, ins.Name, ins.ID)
}

// servicePrefix 由 service 名构造前缀：prefix/<name>/
func (r *etcdRegistry) servicePrefix(name string) string {
	return path.Join(r.prefix, name) + "/"
}

// Register 把实例写入 etcd 并启动 lease 自动续约。
//
// 步骤：
//  1. 序列化 Instance 为 JSON
//  2. 申请 lease（TTL）
//  3. Put key=value, WithLease
//  4. 启动 KeepAlive goroutine
//
// 同一 Instance.ID 重复 Register 会先撤销旧 lease 再注册新 lease，保证幂等。
func (r *etcdRegistry) Register(ctx context.Context, ins *types.Instance) error {
	if ins == nil {
		return fmt.Errorf("etcd: register nil instance")
	}
	if ins.ID == "" || ins.Name == "" {
		return fmt.Errorf("etcd: instance Id and Name are required")
	}

	cli, err := r.getClient()
	if err != nil {
		return err
	}

	payload, err := json.Marshal(ins)
	if err != nil {
		return fmt.Errorf("etcd: marshal instance %s: %w", ins.ID, err)
	}

	// 幂等：若已有旧 lease，先撤销（停止 KeepAlive + revoke lease + 删 key）
	r.revokeExisting(ins.ID)

	// 申请 lease（TTL 秒）
	grantResp, err := cli.Grant(ctx, int64(r.ttl.Seconds()))
	if err != nil {
		return fmt.Errorf("etcd: grant lease for %s: %w", ins.ID, err)
	}
	leaseID := grantResp.ID

	key := r.instanceKey(ins)
	// Put 时关联 lease；若 Put 失败立即 revoke 防止 lease 泄漏
	_, err = cli.Put(ctx, key, string(payload), clientv3.WithLease(leaseID))
	if err != nil {
		// 用独立 ctx：调用方 ctx 可能已被 cancel（如超时），Revoke 仍要执行
		revokeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = cli.Revoke(revokeCtx, leaseID)
		return fmt.Errorf("etcd: put %s: %w", key, err)
	}

	// KeepAlive 必须长期运行直到 Deregister / 进程退出
	kaCtx, kaCancel := context.WithCancel(context.Background())
	go r.keepAlive(kaCtx, leaseID, ins.ID)

	r.mu.Lock()
	r.leases[ins.ID] = &leaseEntry{leaseID: leaseID, cancel: kaCancel}
	r.mu.Unlock()
	return nil
}

// keepAlive 持续续约 lease。KeepAlive 返回的 channel 每次收到响应即续约成功；
// channel 关闭表示 lease 失效或 etcd 连接断开，本 goroutine 退出。
//
// 失效时不主动重新注册（业务场景复杂），交由上层 ServiceComponent 监听到注册失败重试。
func (r *etcdRegistry) keepAlive(ctx context.Context, leaseID clientv3.LeaseID, instanceID string) {
	cli, err := r.getClient()
	if err != nil {
		return
	}
	ch, err := cli.KeepAlive(ctx, leaseID)
	if err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch:
			if !ok {
				// KeepAlive channel 关闭：lease 已失效或 ctx 取消
				return
			}
		}
	}
}

// revokeExisting 撤销指定 Instance.ID 的旧 lease（幂等：不存在则 no-op）
func (r *etcdRegistry) revokeExisting(instanceID string) {
	r.mu.Lock()
	entry, ok := r.leases[instanceID]
	if ok {
		delete(r.leases, instanceID)
	}
	r.mu.Unlock()
	if !ok {
		return
	}
	entry.cancel()
	// Revoke 用独立 ctx：调用方的 ctx 可能已 cancel
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if cli, err := r.getClient(); err == nil {
		_, _ = cli.Revoke(ctx, entry.leaseID)
	}
}

// Deregister 反注册：停止 KeepAlive + revoke lease + delete key。
// revoke lease 会自动让 key 失效，delete 是冗余兜底（防止 etcd 时钟漂移）。
func (r *etcdRegistry) Deregister(ctx context.Context, ins *types.Instance) error {
	if ins == nil || ins.ID == "" {
		return nil // Deregister 设计为幂等 no-op
	}

	cli, err := r.getClient()
	if err != nil {
		return err
	}

	// 先停 KeepAlive + revoke lease（让 etcd 自动删 key，触发订阅者 Watch）
	r.revokeExisting(ins.ID)

	// 兜底：显式 delete
	key := r.instanceKey(ins)
	_, err = cli.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("etcd: delete %s: %w", key, err)
	}
	return nil
}

// GetService 拉取该 service 名下的全部实例并聚合为 *types.ServiceEntry。
//
// 内部步骤：
//  1. WithPrefix 查询 <prefix>/<name>/ 下所有 KV
//  2. 逐个反序列化 value 为 Instance
//  3. 通过 NewServiceEntry + AddInstance 聚合（自动建立 Clusters 二级索引）
func (r *etcdRegistry) GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("etcd: empty service name")
	}

	cli, err := r.getClient()
	if err != nil {
		return nil, err
	}

	prefix := r.servicePrefix(serviceName)
	resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd: get prefix %s: %w", prefix, err)
	}

	entry := types.NewServiceEntry()
	for _, kv := range resp.Kvs {
		var ins types.Instance
		if err := json.Unmarshal(kv.Value, &ins); err != nil {
			// 跳过格式异常的 key（可能其他客户端写入了不兼容格式），不阻塞整体查询
			continue
		}
		_ = entry.AddInstance(&ins)
	}

	if len(entry.Instances) == 0 {
		return nil, fmt.Errorf("etcd: service %q not found", serviceName)
	}
	return entry, nil
}

// Watch 监听服务名下的实例变更，每次变更推送 struct{}{} 到返回 channel。
//
// 设计：
//   - channel 容量 1，避免慢消费方阻塞 etcd Watch goroutine
//   - 首次 Watch 立即触发一次事件（让订阅者拿到初始状态）
//   - ctx 取消时关闭 channel 并清理 watcher
//
// 注意：本方法只发"有变更"信号，订阅者需自行调用 GetService 拿最新列表。
// 这种 coarser-grained 通知简化了订阅者逻辑（无需处理增量事件合并）。
func (r *etcdRegistry) Watch(ctx context.Context, serviceName string) (<-chan struct{}, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("etcd: empty service name")
	}

	cli, err := r.getClient()
	if err != nil {
		return nil, err
	}

	prefix := r.servicePrefix(serviceName)
	ch := make(chan struct{}, 1)

	// 立即推送一次，确保订阅者从最新状态起步
	ch <- struct{}{}

	watcher := clientv3.NewWatcher(cli)
	go func() {
		defer close(ch)
		defer watcher.Close()

		// WatchWithContext 接收外部 ctx：取消时 etcd 自动关闭 watch chan
		watchCh := watcher.Watch(ctx, prefix, clientv3.WithPrefix())
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					return
				}
				if err := resp.Err(); err != nil {
					// 监听错误（如 compaction）：通知订阅者重新建立监听
					select {
					case ch <- struct{}{}:
					default:
					}
					return
				}
				// 非空事件触发通知；空事件（如 Canceled/Compact 已通过 Err()）忽略
				if len(resp.Events) > 0 {
					select {
					case ch <- struct{}{}:
					default:
						// 订阅者还没消费上一条事件，丢弃本次（coalescing 语义）
					}
				}
			}
		}
	}()

	return ch, nil
}

// Close 关闭底层 etcd client（仅在ownsClient=true 时关闭）。
// 可重复调用。
func (r *etcdRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 停掉所有 KeepAlive
	for id, entry := range r.leases {
		entry.cancel()
		delete(r.leases, id)
	}
	if r.client != nil && r.ownsClient {
		err := r.client.Close()
		r.client = nil
		return err
	}
	return nil
}

// Address 返回当前配置的 endpoints（逗号分隔），便于日志诊断。
// 不直接暴露 *clientv3.Client，避免外部修改内部状态。
func (r *etcdRegistry) Address() string {
	return strings.Join(r.endpoints, ",")
}
