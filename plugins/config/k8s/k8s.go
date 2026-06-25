// Package k8s 提供基于 K8s ConfigMap 的 config.Loader 实现。
//
// 设计要点：
//   - 默认 in-cluster 鉴权（运行在 pod 内自动用 ServiceAccount）
//   - 开发模式用 WithKubeconfig("/path/to/kubeconfig") 接外部集群
//   - 监听单个 ConfigMap（WithName）；data 字段每个 key 转为 KeyValue
//   - Watch 用 client-go Watch API，事件到达后重新 Get 返回全量快照
//     行为对齐 config/file 和 config/etcd
//
// 用法：
//
//	loader, err := k8s.New(
//	    k8s.WithName("my-app-config"),
//	    k8s.WithNamespace("default"),
//	)
//	cfg, _ := config.NewConfig(loader)
//	dsn := cfg.Get("database/dsn")  // data["database/dsn"]
//
// ConfigMap YAML 示例：
//
//	apiVersion: v1
//	kind: ConfigMap
//	metadata:
//	  name: my-app-config
//	  namespace: default
//	data:
//	  database/dsn: "postgres://localhost"
//	  feature.flag: "true"
package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-zeus/zeus/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// DefaultNamespace 默认命名空间
	DefaultNamespace = "default"

	// DefaultWatchTimeout Watch 长连接超时（0 = 跟随 server，-1 = 永不超时）
	// K8s 默认 5-30min server-side timeout；client 重新建立连接是常规行为
	DefaultWatchTimeout = 0
)

// Option 函数式选项
type Option func(*loader)

// WithKubeconfig 用 kubeconfig 文件鉴权（开发模式）。
// 不调用且非 in-cluster 时默认尝试 ~/.kube/config。
func WithKubeconfig(path string) Option {
	return func(l *loader) {
		if path != "" {
			l.kubeconfigPath = path
		}
	}
}

// WithNamespace 设置 ConfigMap 所在命名空间（默认 default）。
func WithNamespace(ns string) Option {
	return func(l *loader) {
		if ns != "" {
			l.namespace = ns
		}
	}
}

// WithName 设置监听的 ConfigMap 名称（必填）。
func WithName(name string) Option {
	return func(l *loader) {
		if name != "" {
			l.name = name
		}
	}
}

// WithClient 注入外部 kubernetes.Interface（测试或复用连接池）。
// 注入后 kubeconfig/namespace 配置项忽略；Close 不会关闭 client。
func WithClient(c kubernetes.Interface) Option {
	return func(l *loader) {
		if c != nil {
			l.client = c
			l.ownsClient = false
		}
	}
}

// 编译期检查 loader 实现 config.Loader
var _ config.Loader = (*loader)(nil)

type loader struct {
	// 配置
	kubeconfigPath string
	namespace      string
	name           string

	// 运行时
	client     kubernetes.Interface
	ownsClient bool

	once    sync.Once
	initErr error
}

// New 创建 K8s ConfigMap 加载器。
//
// 鉴权优先级：
//  1. WithClient 注入的 client
//  2. KUBECONFIG 环境变量或 WithKubeconfig 指定的 kubeconfig
//  3. ~/.kube/config（开发）
//  4. in-cluster config（运行在 pod 内时）
//
// 拨号是惰性的，首次 Load/Watch 时才实际建立连接。
func New(opts ...Option) (config.Loader, error) {
	l := &loader{
		namespace:  DefaultNamespace,
		ownsClient: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	if l.name == "" {
		return nil, fmt.Errorf("config/k8s: WithName is required")
	}
	return l, nil
}

// getClient 惰性建立 client（线程安全）
func (l *loader) getClient() (kubernetes.Interface, error) {
	l.once.Do(func() {
		if l.client != nil {
			return
		}

		// 1. in-cluster（pod 内自动用 ServiceAccount）
		if cfg, err := rest.InClusterConfig(); err == nil {
			cli, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				l.initErr = fmt.Errorf("config/k8s: in-cluster client: %w", err)
				return
			}
			l.client = cli
			return
		}

		// 2. kubeconfig 文件
		kubeconfig := l.kubeconfigPath
		if kubeconfig == "" {
			// 默认走 KUBECONFIG env 或 ~/.kube/config
			kubeconfig = os.Getenv("KUBECONFIG")
		}
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		if kubeconfig != "" {
			loadingRules.ExplicitPath = kubeconfig
		} else if home, _ := os.UserHomeDir(); home != "" {
			loadingRules.ExplicitPath = filepath.Join(home, ".kube", "config")
		}
		cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			l.initErr = fmt.Errorf("config/k8s: load kubeconfig: %w", err)
			return
		}
		cli, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			l.initErr = fmt.Errorf("config/k8s: new clientset: %w", err)
			return
		}
		l.client = cli
	})
	return l.client, l.initErr
}

// Load 拉取 ConfigMap 全量 data 字段
func (l *loader) Load() ([]config.KeyValue, error) {
	cli, err := l.getClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cm, err := cli.CoreV1().ConfigMaps(l.namespace).Get(ctx, l.name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // ConfigMap 不存在 → 空 KV（与 file/etcd 行为一致）
		}
		return nil, fmt.Errorf("config/k8s: get ConfigMap %s/%s: %w", l.namespace, l.name, err)
	}

	return configMapToKeyValues(cm), nil
}

// Watch 返回监听器。Next 阻塞直到 ConfigMap 变更事件，返回当前全量快照。
func (l *loader) Watch() (config.Watcher, error) {
	cli, err := l.getClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &watcherImpl{
		l:        l,
		cancel:   cancel,
		eventCh:  make(chan struct{}, 1),
		stopOnce: sync.Once{},
	}

	// 启动 K8s Watch 转发 goroutine
	// client-go 的 Watch 长连接可能被 server 端关闭（默认 30min），
	// 这里用最简单的 resync 策略：watcher 关闭就退出，由调用方重新 Watch
	go w.run(ctx, cli)

	return w, nil
}

// configMapToKeyValues 把 ConfigMap.data + binaryData 转为 KeyValue 切片
// cm 为 nil 时返回 nil（防御性：NotFound 后调用方可能误传 nil）
func configMapToKeyValues(cm *corev1.ConfigMap) []config.KeyValue {
	if cm == nil {
		return nil
	}
	total := len(cm.Data) + len(cm.BinaryData)
	if total == 0 {
		return nil
	}
	kvs := make([]config.KeyValue, 0, total)
	for k, v := range cm.Data {
		kvs = append(kvs, config.KeyValue{Key: k, Value: []byte(v)})
	}
	for k, v := range cm.BinaryData {
		kvs = append(kvs, config.KeyValue{Key: k, Value: v})
	}
	return kvs
}

// watcherImpl 实现 config.Watcher
type watcherImpl struct {
	l        *loader
	cancel   context.CancelFunc
	eventCh  chan struct{}
	stopOnce sync.Once
}

// 编译期检查 watcherImpl 实现 config.Watcher
var _ config.Watcher = (*watcherImpl)(nil)

func (w *watcherImpl) run(ctx context.Context, cli kubernetes.Interface) {
	defer close(w.eventCh)

	for {
		if ctx.Err() != nil {
			return
		}
		// Watch 单个 ConfigMap：用 fieldSelector 精确匹配 metadata.name
		watcher, err := cli.CoreV1().ConfigMaps(w.l.namespace).Watch(
			ctx,
			metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", w.l.name),
			},
		)
		if err != nil {
			// 短退避后重试，避免 K8s 临时不可达时空转
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		// 等待事件或 ctx 取消
		eventCh := watcher.ResultChan()
		stopWatch := false
		for !stopWatch {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return
			case event, ok := <-eventCh:
				if !ok {
					// Watch channel 关闭（server-side timeout）→ 重新建立 Watch
					stopWatch = true
					break
				}
				// 仅 ADD/MODIFY/DELETE 触发，BOOKMARK 忽略
				switch event.Type {
				case "ADDED", "MODIFIED", "DELETED":
					select {
					case w.eventCh <- struct{}{}:
					default:
						// coalescing：订阅者还没消费上一条事件，丢弃本次
					}
				}
			}
		}
	}
}

// Next 阻塞直到收到变更事件，返回当前全量快照
func (w *watcherImpl) Next() ([]config.KeyValue, error) {
	_, ok := <-w.eventCh
	if !ok {
		return nil, nil // watcher 关闭
	}
	return w.l.Load()
}

// Stop 关闭 watcher
func (w *watcherImpl) Stop() error {
	w.stopOnce.Do(func() {
		w.cancel()
	})
	return nil
}
