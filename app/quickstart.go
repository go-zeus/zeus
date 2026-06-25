// Package app 提供 Zeus 应用入口。
//
// 本文件实现 L1/L2 渐进暴露 API：
//   - L1：app.Run(cfg, handler) —— 5 行启动（零配置可用）
//   - L2：cfg.Registry URL scheme 切换注册中心
//
// L1/L2 用户不应感知 Component / Container / Instance 等内部概念。
// 需要细粒度控制时走 L4：components.NewApp(...)
package app

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/database"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/middleware/accesslog"
	"github.com/go-zeus/zeus/middleware/recovery"
	"github.com/go-zeus/zeus/middleware/requestid"
	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/server"
	httpdriver "github.com/go-zeus/zeus/server/http"
)

// ServerResolver L1 入口的协议嗅探器（供 plugins 注册）。
//
// 设计动机：主包零第三方依赖（不直接 import google.golang.org/grpc），
// 但仍要支持 L1 用户传入 *grpc.Server 等非 HTTP handler。
// plugins 通过 init() 调用 RegisterServerResolver 注册自己的实现。
//
// 行为契约：
//   - 如果 handler 类型不匹配，返回 (nil, nil)（让 app.Run 继续尝试下一个 resolver）
//   - 如果 handler 类型匹配但构造失败，返回 (nil, err)（终止 app.Run）
//   - 成功返回 (server, nil)
//
// 例如 plugins/server/grpc:
//
//	func init() {
//	    app.RegisterServerResolver(func(h any, cfg *app.Config) (server.Server, error) {
//	        g, ok := h.(*grpc.Server)
//	        if !ok {
//	            return nil, nil  // 不匹配，让其他 resolver 试
//	        }
//	        return FromGRPC(g, Port(cfg.Port)), nil
//	    })
//	}
type ServerResolver func(handler any, cfg *Config) (server.Server, error)

// serverResolvers 按 RegisterServerResolver 调用顺序保存（map 遍历无序，slice 保证稳定）。
var serverResolvers []ServerResolver

// RegisterServerResolver 注册协议嗅探器，由 plugins 在 init() 中调用。
// 主包零依赖，不直接 import 任何 plugin。
func RegisterServerResolver(r ServerResolver) {
	if r != nil {
		serverResolvers = append(serverResolvers, r)
	}
}

// ServerResolvers 返回已注册的协议嗅探器列表（按注册顺序）。
// 主要用于诊断和测试，业务代码通常不需要直接调用。
func ServerResolvers() []ServerResolver {
	out := make([]ServerResolver, len(serverResolvers))
	copy(out, serverResolvers)
	return out
}

// Config L1/L2 用户可见的应用配置
//
// 仅暴露高频字段。需要更细控制（自定义 middleware/metrics/trace/多 server）时
// 走 L4：components.NewApp(...)
type Config struct {
	// Name 服务名（注册中心的 key），默认 "zeus-service"
	Name string

	// Port 监听端口，默认 8080
	Port int

	// IP 监听 IP，默认自动探测本机非 loopback IP
	IP string

	// Cluster 集群名（路由 key，灰度发布用），默认 "default"
	Cluster string

	// Registry 注册中心 URL scheme：
	//   "" / "memory://" → registry/memory（开发默认）
	//   "etcd://host:2379" → 需要 import _ "github.com/go-zeus/zeus/plugins/registry/etcd"
	//                        并通过 RegisterRegistryResolver 注册解析器
	//
	// 不支持的 scheme 返回 error（避免静默用 memory 误导生产）
	Registry string

	// Cache 可选缓存 URL scheme（保持零配置："" 表示不装配 cache 组件）：
	//   ""                          → 不装配（默认）
	//   "memory://?cleanup=60s"     → cache/memory（需 import _ "github.com/go-zeus/zeus/cache/memory"）
	//   "redis://127.0.0.1:6379/0"  → plugins/cache/redis（需 import _ 该包）
	//
	// L2 设计：cache 是可选业务组件，不进入 L1 默认装配清单。
	// 用户显式指定 URL 时才装配对应实现，避免给"5 行 Hello World"加无用依赖。
	Cache string

	// Database 可选数据库 URL scheme（保持零配置："" 表示不装配 database 组件）：
	//   ""                                       → 不装配（默认）
	//   "mysql://user:pass@host:3306/db?pool=50" → plugins/database/mysql（需 import _ 该包）
	//   "postgres://..."                          → plugins/database/postgres（待补）
	//
	// L2 设计：与 Cache 同理。L2 用户通过 cache.NewFromURL/database.NewFromURL 等函数
	// 可在任意层级调用（不仅限于 Config）。
	Database string

	// MQ 可选消息队列 URL scheme（保持零配置："" 表示不装配 mq 组件）：
	//   ""                → 不装配（默认）
	//   "memory://"       → mq/memory（需 import _ "github.com/go-zeus/zeus/mq/memory"）
	//   "kafka://..."     → plugins/mq/kafka（待补）
	MQ string
}

// Run L1 入口：零配置或少量配置启动一个微服务应用
//
// 内部默认装配（用户零感知）：
//   - HTTP server（按 handler 类型推断协议；gRPC 需 import plugins/server/grpc）
//   - slog logger（输出 stdout）
//   - 中间件链（仅 HTTP）：requestid → accesslog → recovery（自动包装 handler）
//   - 注册中心（按 cfg.Registry URL，默认 memory）
//   - 服务注册（按 cfg.Name + cfg.Cluster）
//   - 优雅关闭（SIGTERM/SIGINT/SIGQUIT，10s 超时）
//
// handler 类型支持：
//   - http.Handler / http.HandlerFunc / func(http.ResponseWriter, *http.Request) → HTTP server
//   - nil（用 server 内置默认 HTTP handler）
//   - 其他类型（如 *grpc.Server）：交由 RegisterServerResolver 注册的协议嗅探器
//     主包不直接依赖 grpc，需 import _ "github.com/go-zeus/zeus/plugins/server/grpc"
//
// 5 行 Hello World：
//
//	func main() {
//	    app.Run(&app.Config{Port: 8080}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        w.Write([]byte("hello"))
//	    }))
//	}
//
// 返回值：应用退出错误（信号触发正常关闭时返回 nil）。
func Run(cfg *Config, handler any) error {
	if cfg == nil {
		cfg = &Config{}
	}
	applyDefaults(cfg)

	srv, _, err := resolveServer(handler, cfg)
	if err != nil {
		return err
	}

	reg, err := resolveRegistry(cfg.Registry)
	if err != nil {
		return err
	}

	// 装配 components（顺序不影响，容器会拓扑排序）
	var comps []any
	comps = append(comps,
		// 日志：默认 slog → stdout
		components.NewLogComponent(logslog.NewSlog()),
		// 注册中心
		components.NewRegistryComponent(reg),
		// Server
		components.NewServerComponent(srv),
		// 服务注册
		components.NewServiceComponent(serviceOptions(cfg)...),
	)

	// 可选组件（cache/database/mq）：仅当 Config 指定 URL 时装配
	if err := appendOptionalComponents(&comps, cfg); err != nil {
		return err
	}

	return components.NewApp(comps...).Run()
}

// appendOptionalComponents 按 Config 中的可选 URL 装配 cache/database/mq 组件。
//
// 设计原则：
//   - L1/L2 默认零配置：URL 为空时跳过（不强制用户引入第三方依赖）
//   - URL 解析失败立即返回 error（避免静默忽略误导生产）
//   - tracer/meter 默认 nil，由实现包兜底 noop（与 L4 显式装配行为一致）
func appendOptionalComponents(comps *[]any, cfg *Config) error {
	if cfg.Cache != "" {
		c, err := cache.NewFromURL(cfg.Cache)
		if err != nil {
			return fmt.Errorf("app.Run: resolve cache URL: %w", err)
		}
		*comps = append(*comps, components.NewCacheComponent(c))
	}

	if cfg.Database != "" {
		// L2 入口暂不注入 tracer/meter（保持 L1 心智：5 行启动）
		// 需要 trace/metrics 集成时走 L3：app.WithDatabaseURL 或 L4 手动装配
		db, err := database.NewFromURL(cfg.Database, nil, nil)
		if err != nil {
			return fmt.Errorf("app.Run: resolve database URL: %w", err)
		}
		*comps = append(*comps, components.NewDatabaseComponent(db))
	}

	if cfg.MQ != "" {
		broker, err := mq.NewBrokerFromURL(cfg.MQ)
		if err != nil {
			return fmt.Errorf("app.Run: resolve mq URL: %w", err)
		}
		*comps = append(*comps, components.NewMQComponent(broker))
	}

	return nil
}

// resolveServer 按 handler 类型选择 server 实现。
//
// 顺序：
//  1. http.Handler / http.HandlerFunc / func(...) / nil → 内置 HTTP server
//     + 自动包装 accesslog/requestid/recovery 中间件链
//  2. 遍历 RegisterServerResolver 注册的协议嗅探器，第一个返回非 nil 的胜出
//     （gRPC 等非 HTTP 协议走这条路径，主包零依赖）
//  3. 全部未命中 → 返回明确错误（避免静默用错类型）
//
// 返回的 kind 字符串仅用于日志和错误信息，"http" 或 resolver 自报。
func resolveServer(handler any, cfg *Config) (server.Server, string, error) {
	// 1. HTTP 内置检测
	httpHandler, coerceErr := coerceHandler(handler)
	if coerceErr == nil {
		// 命中 HTTP 类型（含 nil：让 server 用默认 handler）
		wrapped := wrapHTTPMiddleware(httpHandler)
		return newHTTPServer(cfg, wrapped), "http", nil
	}

	// 2. 遍历注册的协议嗅探器
	for _, r := range serverResolvers {
		s, err := r(handler, cfg)
		if err != nil {
			return nil, "", err
		}
		if s != nil {
			return s, "plugin", nil
		}
	}

	// 3. 都不匹配
	return nil, "", fmt.Errorf("app.Run: unsupported handler type %T "+
		"(expected http.Handler / http.HandlerFunc / func(http.ResponseWriter, *http.Request) / nil, "+
		"or import the corresponding plugin for gRPC etc.); original coerce error: %w",
		handler, coerceErr)
}

// wrapHTTPMiddleware 给 HTTP handler 包装默认中间件链。
//
// 顺序（外→内）：requestid → accesslog → recovery → user handler
// 包装顺序说明：最先调用的 HTTPMiddleware 是最内层，最后调用的 HTTPMiddleware 是最外层。
// 因此 requestid 必须最后包装（成为最外层），accesslog 才能从 ctx 读到 request id。
func wrapHTTPMiddleware(httpHandler http.Handler) http.Handler {
	if httpHandler == nil {
		return nil
	}
	chain := middleware.NewChain(recovery.New())
	// 1. 内层：accesslog 包装 user handler（先包，最后被调用）
	httpHandler = accesslog.HTTPMiddleware(httpHandler)
	// 2. 外层：requestid 包装（最后包，最先被调用，注入 ctx 给 accesslog 用）
	httpHandler = requestid.HTTPMiddleware(httpHandler)
	// 3. 最外层：recovery（防 panic）
	return httpdriver.ChainHandler(httpHandler, chain)
}

// applyDefaults 填充 Config 默认值（不允许默认失败原则）
func applyDefaults(cfg *Config) {
	if cfg.Name == "" {
		cfg.Name = "zeus-service"
	}
	if cfg.Port == 0 {
		cfg.Port = httpdriver.DefaultPort
	}
	if cfg.Cluster == "" {
		cfg.Cluster = "default"
	}
}

// coerceHandler 把多种 handler 形态统一为 http.Handler
// nil 时返回 nil，由 server 自行使用默认 handler
func coerceHandler(handler any) (http.Handler, error) {
	switch h := handler.(type) {
	case nil:
		return nil, nil
	case http.Handler:
		// 覆盖 *ServeMux / http.HandlerFunc / 自定义实现 http.Handler 的类型
		return h, nil
	case func(http.ResponseWriter, *http.Request):
		// 普通函数字面量（未显式转换为 http.HandlerFunc）
		return http.HandlerFunc(h), nil
	default:
		return nil, fmt.Errorf("app.Run: unsupported handler type %T (expected http.Handler / http.HandlerFunc / func(http.ResponseWriter, *http.Request) / nil)", handler)
	}
}

// newHTTPServer 按 Config + handler 构造 HTTP server
func newHTTPServer(cfg *Config, handler http.Handler) server.Server {
	opts := []httpdriver.Option{httpdriver.Port(cfg.Port)}
	if cfg.IP != "" {
		opts = append(opts, httpdriver.IP(cfg.IP))
	}
	if handler != nil {
		opts = append(opts, httpdriver.Mux(handler))
	}
	return httpdriver.NewHTTP(opts...)
}

// serviceOptions 把 Config 映射到 ServiceComponent 选项
func serviceOptions(cfg *Config) []components.ServiceOption {
	opts := []components.ServiceOption{
		components.WithServiceName(cfg.Name),
		components.WithServiceCluster(cfg.Cluster),
	}
	if cfg.IP != "" {
		opts = append(opts, components.WithServiceIP(cfg.IP))
	}
	return opts
}

// RegistryResolver 注册中心 URL 解析器（供 plugins 注册）
//
// L1 不直接依赖 plugins/registry/etcd 等第三方包，
// plugins 通过 init() 调用 RegisterRegistryResolver 注册自己的 scheme。
// 例如 etcd plugin:
//
//	func init() {
//	    app.RegisterRegistryResolver("etcd", func(rawURL string) (registry.Registrar, error) {
//	        // 解析 etcd://host:2379 并构造 etcd registry
//	    })
//	}
type RegistryResolver func(rawURL string) (registry.Registrar, error)

var registryResolvers = map[string]RegistryResolver{}

// RegisterRegistryResolver 注册 URL scheme → Resolver
// 由 plugins 在 init() 中调用，主包零依赖
func RegisterRegistryResolver(scheme string, r RegistryResolver) {
	if scheme != "" && r != nil {
		registryResolvers[scheme] = r
	}
}

// resolveRegistry 解析 cfg.Registry URL
//
// "" / "memory://" → memory.New()
// "<scheme>://..." → 查找 RegisterRegistryResolver 注册的解析器
// 未知 scheme → error（避免静默用 memory 误导生产）
func resolveRegistry(url string) (registry.Registrar, error) {
	if url == "" || url == "memory://" {
		return memory.New(), nil
	}

	scheme := url
	if idx := strings.Index(url, "://"); idx > 0 {
		scheme = url[:idx]
	}

	resolver, ok := registryResolvers[scheme]
	if !ok {
		return nil, fmt.Errorf("app.Run: unknown registry scheme %q (import the corresponding plugin or use \"memory://\")", scheme)
	}
	return resolver(url)
}
