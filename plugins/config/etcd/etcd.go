// Package etcd 提供基于 go.etcd.io/etcd/client/v3 的 config.Loader 实现。
//
// 设计要点：
//   - 复用 etcd client v3（与 plugins/registry/etcd 同一依赖）
//   - Load 用 Get WithPrefix 拉全量 KV，key 去掉前缀返回
//   - Watch 用 etcd Watcher 推送事件，Next 在事件到达后重新拉全量
//     行为对齐 config/file：每次 Next 返回当前快照（不是增量）
//   - 支持 prefix 模式（多 KV）和 key 模式（单 KV）
//
// 默认 prefix：/zeus/config/
//
// 用法：
//
//	loader := etcd.New(
//	    etcd.WithEndpoints("127.0.0.1:2379"),
//	    etcd.WithPrefix("/myapp/"),
//	)
//	cfg, err := config.NewConfig(loader)
//	value := cfg.Get("database/dsn")   // key 为去掉 prefix 后的相对路径
package etcd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-zeus/zeus/config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	// DefaultEndpoint 用户可覆盖
	DefaultEndpoint = "127.0.0.1:2379"

	// DefaultDialTimeout 默认拨号超时（etcd v3 内部 gRPC，包含 TLS+HTTP/2 协商）
	DefaultDialTimeout = 30 * time.Second

	// DefaultPrefix 默认配置 key 前缀
	// 与 registry 的 /zeus/services/ 区分，留出 /zeus/config /zeus/cluster 等扩展空间
	DefaultPrefix = "/zeus/config/"
)

// Option 函数式选项
type Option func(*loader)

// WithEndpoints 设置一个或多个 etcd 集群节点地址（host:port）。
func WithEndpoints(endpoints ...string) Option {
	return func(l *loader) {
		if len(endpoints) > 0 {
			l.endpoints = append([]string(nil), endpoints...)
		}
	}
}

// WithPrefix 设置 prefix 模式：加载 <prefix>/* 下所有 KV，key 去掉 prefix 返回。
// 与 WithKey 互斥；同时设置时 WithKey 优先。
func WithPrefix(prefix string) Option {
	return func(l *loader) {
		if prefix != "" {
			l.prefix = prefix
		}
	}
}

// WithKey 设置 key 模式：加载单 key。
// 与 WithPrefix 互斥；同时设置时 WithKey 优先。
func WithKey(key string) Option {
	return func(l *loader) {
		if key != "" {
			l.key = key
		}
	}
}

// WithDialTimeout 设置首次连接 etcd 的拨号超时。
func WithDialTimeout(d time.Duration) Option {
	return func(l *loader) {
		if d > 0 {
			l.dialTimeout = d
		}
	}
}

// WithCredentials 启用 etcd 用户名/密码鉴权。
func WithCredentials(username, password string) Option {
	return func(l *loader) {
		l.username = username
		l.password = password
	}
}

// WithClient 注入外部 *clientv3.Client，跳过本包拨号。
// 注入后 Stop 不会关闭该 client（由外部所有者负责）。
func WithClient(cli *clientv3.Client) Option {
	return func(l *loader) {
		l.client = cli
		l.ownsClient = false
	}
}

// 编译期检查 loader 实现 config.Loader
var _ config.Loader = (*loader)(nil)

type loader struct {
	// 配置
	endpoints   []string
	prefix      string
	key         string
	dialTimeout time.Duration
	username    string
	password    string

	// 运行时
	client     *clientv3.Client
	ownsClient bool

	mu sync.Mutex
}

// New 创建 etcd 配置加载器。
//
// 默认值：
//   - Endpoints: [DefaultEndpoint]
//   - Prefix: /zeus/config/
//   - DialTimeout: 30s
//
// 拨号是惰性的，首次 Load/Watch 时才实际建立连接。
func New(opts ...Option) config.Loader {
	l := &loader{
		endpoints:   []string{DefaultEndpoint},
		prefix:      DefaultPrefix,
		dialTimeout: DefaultDialTimeout,
		ownsClient:  true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// getClient 惰性建立 client（线程安全）
func (l *loader) getClient() (*clientv3.Client, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.client != nil {
		return l.client, nil
	}
	var (
		cli *clientv3.Client
		err error
	)
	cfg := clientv3.Config{
		Endpoints:   l.endpoints,
		DialTimeout: l.dialTimeout,
	}
	if l.username != "" || l.password != "" {
		cfg.Username = l.username
		cfg.Password = l.password
	}
	if cli, err = clientv3.New(cfg); err != nil {
		return nil, fmt.Errorf("config/etcd: dial %v: %w", l.endpoints, err)
	}
	l.client = cli
	return cli, nil
}

// watchTarget 返回当前监听目标（key 优先于 prefix）和是否 prefix 模式
func (l *loader) watchTarget() (target string, isPrefix bool) {
	if l.key != "" {
		return l.prefix + strings.TrimPrefix(l.key, "/"), false
	}
	return l.prefix, true
}

// Load 拉取配置全量 KV
func (l *loader) Load() ([]config.KeyValue, error) {
	cli, err := l.getClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	target, isPrefix := l.watchTarget()
	getOpts := []clientv3.OpOption{}
	if isPrefix {
		getOpts = append(getOpts, clientv3.WithPrefix())
	}
	resp, err := cli.Get(ctx, target, getOpts...)
	if err != nil {
		return nil, fmt.Errorf("config/etcd: get %s: %w", target, err)
	}

	kvs := make([]config.KeyValue, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		// 去掉 prefix，返回相对 key（业务侧使用相对路径访问）
		relKey := strings.TrimPrefix(string(kv.Key), l.prefix)
		kvs = append(kvs, config.KeyValue{Key: relKey, Value: kv.Value})
	}
	return kvs, nil
}

// Watch 返回监听器。Next 阻塞直到 etcd 推送变更事件，然后重新拉全量返回快照。
func (l *loader) Watch() (config.Watcher, error) {
	cli, err := l.getClient()
	if err != nil {
		return nil, err
	}
	target, isPrefix := l.watchTarget()

	ctx, cancel := context.WithCancel(context.Background())
	watcher := clientv3.NewWatcher(cli)
	var watchCh clientv3.WatchChan
	if isPrefix {
		watchCh = watcher.Watch(ctx, target, clientv3.WithPrefix())
	} else {
		watchCh = watcher.Watch(ctx, target)
	}

	w := &watcherImpl{
		l:        l,
		watcher:  watcher,
		ctx:      ctx,
		cancel:   cancel,
		eventCh:  make(chan struct{}, 1),
		stopOnce: sync.Once{},
	}

	// 启动 etcd watch 转发 goroutine：把 etcd 事件合并到 eventCh
	go func() {
		defer close(w.eventCh)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					return
				}
				if resp.Err() != nil {
					// compaction 等错误：触发一次重新拉取
					select {
					case w.eventCh <- struct{}{}:
					default:
					}
					return
				}
				if len(resp.Events) > 0 {
					select {
					case w.eventCh <- struct{}{}:
					default:
						// coalescing：订阅者还没消费上一次事件，丢弃本次
					}
				}
			}
		}
	}()

	return w, nil
}

// watcherImpl 实现 config.Watcher
type watcherImpl struct {
	l        *loader
	watcher  clientv3.Watcher
	ctx      context.Context
	cancel   context.CancelFunc
	eventCh  chan struct{}
	stopOnce sync.Once
}

// 编译期检查 watcherImpl 实现 config.Watcher
var _ config.Watcher = (*watcherImpl)(nil)

// Next 阻塞直到收到变更事件，返回当前全量快照
func (w *watcherImpl) Next() ([]config.KeyValue, error) {
	select {
	case <-w.ctx.Done():
		return nil, nil
	case _, ok := <-w.eventCh:
		if !ok {
			return nil, nil // watcher 关闭
		}
		// 收到事件 → 重新拉全量
		return w.l.Load()
	}
}

// Stop 关闭 watcher
func (w *watcherImpl) Stop() error {
	var err error
	w.stopOnce.Do(func() {
		w.cancel()
		err = w.watcher.Close()
		// 若 loader 拥有 client，Stop 时不关闭（loader 生命周期更长）
	})
	return err
}

// ErrNoConfig 没有配置时的错误
var ErrNoConfig = errors.New("config/etcd: no keys found")
