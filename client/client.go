package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/propagation"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/routing"
)

// HTTPClient HTTP 协议客户端接口
//
// 设计说明：主包 client 仅提供 HTTP 协议的客户端。
// gRPC 等其他协议的客户端走 plugins/client/<protocol> 独立 module，
// 不强行抽象为统一接口——HTTP/gRPC 请求模型本质不同（method+url+body vs method+protobuf），
// 强行抽象会失去类型安全（违反 KISS）。
type HTTPClient interface {
	Do(r *http.Request) (*http.Response, error)
}

// Client 兼容别名：等价于 HTTPClient
//
// Deprecated: 使用 HTTPClient 代替，明确表示这是 HTTP 协议专用客户端
type Client = HTTPClient

type client struct {
	name       string
	dis        registry.Discovery
	lb         balancer.Balancer
	clustersMu sync.RWMutex
	clusters   map[string]balancer.Balancer
	cc         *http.Client
	ownsClient bool // 是否拥有 cc 的所有权（决定是否在 Close 时关闭连接池）
	stopCh     chan struct{}
	closeOnce  sync.Once
	tlsCfg     *tls.Config     // TLS 配置（注入到 Transport）
	transport  *http.Transport // 自定义 Transport（连接池调优）
	timeout    time.Duration   // http.Client 全局超时
}

type Option func(c *client)

// WithHTTPClient 注入自定义 http.Client（默认会构造独立的 http.Client）
// 注入后 Close 不会关闭其连接池，由调用方管理
func WithHTTPClient(hc *http.Client) Option {
	return func(c *client) {
		c.cc = hc
		c.ownsClient = false
	}
}

// WithTLS 配置 TLS（用于 HTTPS 调用）。
//
// 行为：
//   - 在默认 http.Client 的 Transport 上注入 TLSClientConfig
//   - 若用户已通过 WithHTTPClient 注入自定义 Client，则修改其 Transport（要求是 *http.Transport）
//   - mTLS 场景请传入含 Certificates + ClientCAs + ClientAuth 的 *tls.Config
//
// 示例：
//
//	c := client.NewClient("api-svc",
//	    client.WithTLS(&tls.Config{InsecureSkipVerify: true}),
//	)
func WithTLS(cfg *tls.Config) Option {
	return func(c *client) {
		c.tlsCfg = cfg
	}
}

// WithTransport 自定义 http.Transport（用于连接池调优）。
//
// 适用场景：
//   - 调整 MaxIdleConns / MaxIdleConnsPerHost / IdleConnTimeout
//   - 启用 HTTP/2（Transport.ForceAttemptHTTP2 = true）
//   - 自定义 DialContext / Proxy
//
// 注意：调用此 Option 会替换默认 Transport；若同时调用 WithTLS，
// TLS 配置会自动写入新 Transport 的 TLSClientConfig。
func WithTransport(rt *http.Transport) Option {
	return func(c *client) {
		c.transport = rt
	}
}

// WithTimeout 设置 http.Client 全局请求超时
//
// 默认 0（无超时，依赖 ctx 控制）
func WithTimeout(d time.Duration) Option {
	return func(c *client) {
		c.timeout = d
	}
}

// NewClient 创建 HTTP 客户端
//
// 内部启动 watcher goroutine 监听服务发现变更，调用方应在不再使用时调用 Close 释放资源
func NewClient(name string, opts ...Option) HTTPClient {
	c := &client{
		name:       name,
		cc:         &http.Client{}, // 独立连接池，避免共享 http.DefaultClient 导致资源管理混乱
		ownsClient: true,
		clusters:   make(map[string]balancer.Balancer),
		stopCh:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	// 应用 TLS / Transport 配置到 http.Client
	c.applyTransportSettings()
	if c.timeout > 0 {
		c.cc.Timeout = c.timeout
	}
	c.load()
	go c.watcher()
	return c
}

// applyTransportSettings 把 WithTLS / WithTransport 应用到 c.cc
//
// 处理顺序：
//  1. 若用户注入自定义 *http.Transport（WithTransport），优先使用
//  2. 否则若 cc.Transport 已是 *http.Transport（注入的 http.Client 带 Transport），用现有
//  3. 否则构造新的 *http.Transport
//  4. 最后若有 tlsCfg，注入到 Transport.TLSClientConfig
func (c *client) applyTransportSettings() {
	if c.cc == nil {
		return
	}
	var tr *http.Transport
	if c.transport != nil {
		tr = c.transport
	} else if existing, ok := c.cc.Transport.(*http.Transport); ok {
		tr = existing
	}
	if tr == nil {
		tr = &http.Transport{}
	}
	if c.tlsCfg != nil {
		tr.TLSClientConfig = c.tlsCfg
	}
	c.cc.Transport = tr
}

func (c *client) watcher() {
	watcher, ok := c.dis.(registry.Watcher)
	if !ok {
		return
	}
	ch, err := watcher.Watch(context.Background(), c.name)
	if err != nil {
		log.Info("client %s: watch failed: %v", c.name, err)
		return
	}
	for {
		select {
		case <-c.stopCh:
			return
		case <-ch:
			c.load()
		}
	}
}

// Close 关闭客户端，停止 watcher goroutine 并清理自有的 http.Client 连接池
// 支持多次调用不 panic
func (c *client) Close() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
		if c.ownsClient && c.cc != nil {
			if tr, ok := c.cc.Transport.(*http.Transport); ok {
				tr.CloseIdleConnections()
			}
		}
	})
}

func (c *client) load() {
	if c.dis == nil {
		return
	}
	srv, err := c.dis.GetService(context.Background(), c.name)
	if err != nil {
		log.Info("client %s: get service failed: %v", c.name, err)
		return
	}
	if srv == nil {
		return
	}
	newClusters := make(map[string]balancer.Balancer)
	if c.lb == nil {
		// 未配置 balancer，无法路由（避免静默产生空候选集）
		log.Error("client %s: balancer not configured, skip reload", c.name)
		return
	}
	for name, cl := range srv.Clusters {
		instances := cl.GetInstances()
		if len(instances) == 0 {
			continue
		}
		newClusters[name] = c.lb.Reload(instances)
	}
	c.clustersMu.Lock()
	c.clusters = newClusters
	c.clustersMu.Unlock()
}

func (c *client) Do(r *http.Request) (res *http.Response, err error) {
	if r == nil || r.URL == nil {
		return nil, fmt.Errorf("client: invalid request")
	}

	cluster := resolveCluster(r)

	// 把 cluster 写回 header（若未显式设置），保证下游 srv 的 clusterInjector 能解析到
	// 否则 cluster 仅存在于 ctx，HTTP 转发后丢失，下游会读到 default
	if r.Header.Get(routing.HeaderCluster) == "" && cluster != "" {
		r.Header.Set(routing.HeaderCluster, cluster)
	}

	// 自动注入 W3C Baggage Header：把 ctx 中所有 K-V（含 zeus.cluster 和用户自定义）
	// 透传给下游，下游的 clusterInjector 会自动 extract 到 ctx
	// 用户在业务代码调 propagation.With(ctx, "tenant.id", "acme") 后即可全链路传播
	propagation.InjectHTTP(r.Context(), r.Header)

	c.clustersMu.RLock()
	lb, ok := c.clusters[cluster]
	if !ok {
		// fallback 到 default 集群
		lb, ok = c.clusters[routing.Default]
	}
	c.clustersMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("client: no available cluster (tried %q and default)", cluster)
	}

	ins, err := lb.Next()
	if err != nil {
		return nil, err
	}

	scheme := r.URL.Scheme
	if scheme == "" {
		scheme = "http"
	}
	// 保留原始 Path 和 Query，仅替换 host
	r.URL, err = url.Parse(fmt.Sprintf("%s://%s:%d%s", scheme, ins.IP, ins.Port, r.URL.RequestURI()))
	if err != nil {
		return nil, err
	}
	return c.cc.Do(r)
}

// resolveCluster 从 HTTP Header 和 context 解析集群标记
func resolveCluster(r *http.Request) string {
	// 优先从 Header 读取
	if cluster := r.Header.Get(routing.HeaderCluster); cluster != "" {
		return cluster
	}
	// 其次从 context 读取
	return routing.FromContext(r.Context())
}
