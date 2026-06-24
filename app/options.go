package app

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-zeus/zeus/cache"
	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/metrics"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/server"
	zeushttp "github.com/go-zeus/zeus/server/http"
	"github.com/go-zeus/zeus/trace"
)

// —— L3 用户可见的默认值（与 L1 保持一致） ——

const (
	defaultServiceName    = "zeus-service"
	defaultServiceCluster = "default"
	defaultStopTimeoutL3  = 10 * time.Second
)

// appConfig L3 装配的内部状态（用户不直接操作，由 WithXxx 设置）
type appConfig struct {
	name        string
	cluster     string
	ip          string
	servers     []server.Server
	registry    registry.Registrar
	logger      log.Writer
	meter       metrics.Meter
	tracer      trace.Tracer
	middlewares []middleware.Interceptor
	stopTimeout time.Duration

	// 可选业务组件的 URL（保持 L3 类型装配心智：用户传 URL 字符串走 NewFromURL 解析）
	// 与 WithComponent(components.NewCacheComponent(...)) 的差异：
	//   - WithCacheURL：用户只需配置 URL，由 cache.NewFromURL 解析（隐藏实现细节）
	//   - WithComponent：用户自己构造实例（L4 风格，类型装配）
	cacheURL    string
	databaseURL string
	mqURL       string

	// extraComps 用户追加的 L4 组件（透传给 components.NewApp）
	// 用 any 类型，可同时接受 Component 或 components.AppOption
	extraComps []any
}

// AppOption L3 类型装配选项
type AppOption func(*appConfig)

// AddServer 追加 Server（可多次调用实现多 Server 场景）。
//
// 命名说明：使用 Add 而非 With 是为了与 L4 旧入口 app.WithServer（覆盖式）区分；
// L3 AddServer 是追加式（多次调用累加），语义更准确。
//
// 行为：
//   - 多次调用累加到 servers 列表
//   - 每个 Server 都会注册一个 Instance（共享 Name，区分 Protocol）
//   - 至少需要一次调用，否则 Run 时 ServiceComponent 依赖检查失败
func AddServer(srv server.Server) AppOption {
	return func(c *appConfig) {
		if srv != nil {
			c.servers = append(c.servers, srv)
		}
	}
}

// WithLogger 设置日志 Writer（默认 slog → stdout，与 L1 一致）
func WithLogger(w log.Writer) AppOption {
	return func(c *appConfig) {
		if w != nil {
			c.logger = w
		}
	}
}

// WithRegistry 设置注册中心（默认 memory.New()，与 L1 一致）
//
// 注意：L3 不做 URL scheme 解析（用户已 import 实现包）；
// 如需 URL 切换走 L1/L2 的 Config.Registry 字段
func WithRegistry(r registry.Registrar) AppOption {
	return func(c *appConfig) {
		if r != nil {
			c.registry = r
		}
	}
}

// WithMeter 设置 Meter（默认不装配，各组件 noop 兜底）
//
// 注入 prometheus 等实现后，业务可观测性自动接入（中间件 / db / cache 等）
func WithMeter(m metrics.Meter) AppOption {
	return func(c *appConfig) {
		if m != nil {
			c.meter = m
		}
	}
}

// WithTracer 设置 Tracer（默认不装配，各组件 noop 兜底）
//
// 注入 otel 等实现后，自动产生 db.query / cache.get 等链路 span
func WithTracer(t trace.Tracer) AppOption {
	return func(c *appConfig) {
		if t != nil {
			c.tracer = t
		}
	}
}

// WithMiddleware 追加全局中间件（可多次调用）。
//
// 行为：
//   - 每次调用为独立 MiddlewareComponent（Name="middleware_<mw.Name()>"）
//   - ServerComponent.OnStart 自动收集所有 MiddlewareComponent，组成 Chain
//   - 多个 middleware 的生效顺序按 Container 拓扑排序（同层按字典序）
//
// 与 L1 的差异：
//   - L1 自动包装 recovery/accesslog/requestid（handler 拦截）
//   - L3 不自动包装，用户需显式调用 WithMiddleware(recovery.New())
//   - 原因：L3 用户已直接构造 server，对中间件链有完全控制
func WithMiddleware(mw middleware.Interceptor) AppOption {
	return func(c *appConfig) {
		if mw != nil {
			c.middlewares = append(c.middlewares, mw)
		}
	}
}

// WithServiceName 设置服务名（注册中心 key），默认 "zeus-service"
func WithServiceName(name string) AppOption {
	return func(c *appConfig) {
		if name != "" {
			c.name = name
		}
	}
}

// WithServiceCluster 设置集群名（路由 key，灰度发布用），默认 "default"
func WithServiceCluster(cluster string) AppOption {
	return func(c *appConfig) {
		if cluster != "" {
			c.cluster = cluster
		}
	}
}

// WithServiceIP 设置服务监听 IP（默认自动探测本机非 loopback IP）
//
// 一般不需要显式设置；多网卡场景可指定具体网卡
func WithServiceIP(ip string) AppOption {
	return func(c *appConfig) {
		if ip != "" {
			c.ip = ip
		}
	}
}

// WithStopTimeout 设置优雅关闭超时（默认 10s）
func WithStopTimeout(d time.Duration) AppOption {
	return func(c *appConfig) {
		if d > 0 {
			c.stopTimeout = d
		}
	}
}

// WithComponent 追加任意 L4 Component 或 components.AppOption
//
// 用途：L3 与 L4 混用场景。用户大部分用 WithXxx，需要补充特殊组件时直接追加：
//
//	app.NewApp(
//	    app.AddServer(http.NewHTTP()),
//	    app.WithComponent(components.NewCacheComponent(cache)),  // L4 透传
//	)
//
// 也可直接作为 NewApp variadic 参数（无需 WithComponent 包装）：
//
//	app.NewApp(
//	    app.AddServer(http.NewHTTP()),
//	    components.NewCacheComponent(cache),  // 直接追加
//	)
func WithComponent(comp any) AppOption {
	return func(c *appConfig) {
		c.extraComps = append(c.extraComps, comp)
	}
}

// WithCacheURL 通过 URL 字符串装配 cache 组件。
//
// 用途：L3 用户希望像 L2 一样通过配置驱动（不必 import 实现包）时使用。
//
// 等价于：cache.NewFromURL(url) → components.NewCacheComponent(c)
//
// 与 WithComponent(components.NewCacheComponent(...)) 的差异：
//   - WithCacheURL：URL 字符串 + 隐式 import _ "plugins/cache/<vendor>" 注册 resolver
//   - WithComponent：用户显式构造实例（L4 风格）
//
// 用法：
//
//	app.NewApp(
//	    app.AddServer(http.NewHTTP()),
//	    app.WithCacheURL("redis://127.0.0.1:6379/0"),
//	)
//
// URL 解析失败时 NewApp 会 panic（与 WithLogger 等选项保持一致：装配错误是开发期问题）
func WithCacheURL(url string) AppOption {
	return func(c *appConfig) {
		if url != "" {
			c.cacheURL = url
		}
	}
}

// WithDatabaseURL 通过 URL 字符串装配 database 组件。
//
// 用途：与 WithCacheURL 同理，但 database 还需 tracer/meter 注入。
// 当前实现：tracer/meter 由 appConfig 内已注入的实例自动透传（用户先调 WithTracer/WithMeter）。
//
// 用法：
//
//	app.NewApp(
//	    app.AddServer(http.NewHTTP()),
//	    app.WithTracer(otelTracer),
//	    app.WithMeter(promMeter),
//	    app.WithDatabaseURL("mysql://user:pass@127.0.0.1:3306/test?pool=50"),
//	)
func WithDatabaseURL(url string) AppOption {
	return func(c *appConfig) {
		if url != "" {
			c.databaseURL = url
		}
	}
}

// WithMQURL 通过 URL 字符串装配 mq 组件。
//
// 用法：
//
//	app.NewApp(
//	    app.AddServer(http.NewHTTP()),
//	    app.WithMQURL("memory://"), // 进程内事件总线
//	)
func WithMQURL(url string) AppOption {
	return func(c *appConfig) {
		if url != "" {
			c.mqURL = url
		}
	}
}

// WithPprof 启用 pprof 诊断端点（独立端口）。
//
// 行为：追加一个独立的 HTTP server 到应用，仅注册 /debug/pprof/* 端点：
//   - GET /debug/pprof/             索引页（列出可用 profile）
//   - GET /debug/pprof/cmdline      命令行参数
//   - GET /debug/pprof/profile      CPU profile（默认 30s 采样）
//   - GET /debug/pprof/symbol       函数符号表
//   - GET /debug/pprof/trace        执行跟踪
//   - GET /debug/pprof/{heap,goroutine,allocs,block,mutex}  各类内存/同步 profile
//
// 设计选择：独立端口而非挂载到业务 mux
//   - 安全隔离：生产环境可在防火墙/网关层屏蔽 pprof 端口（避免业务流量暴露诊断信息）
//   - 性能隔离：pprof 采样本身有开销，独立端口可单独限流
//   - 业界标杆：go-zero / kratos / kitex 均采用独立端口
//
// 用法：
//
//	app.NewApp(
//	    app.AddServer(http.NewHTTP(http.Port(8080), http.Mux(handler))),
//	    app.WithPprof(6060), // pprof 端口
//	)
//
// 默认行为：
//   - 监听 0.0.0.0:<port>
//   - 不注册到服务中心（不生成 Instance）
//   - 不参与集群路由（WithoutAutoClustering）
//
// 生产建议：
//   - 仅在内部环境暴露（防火墙限制源 IP）
//   - 或加 HTTP Basic Auth 中间件保护
func WithPprof(port int) AppOption {
	return func(c *appConfig) {
		if port <= 0 {
			return
		}
		c.servers = append(c.servers, newPprofServer(port))
	}
}

// newPprofServer 构造 pprof 专用 server（独立 mux，注册所有 pprof 端点）
func newPprofServer(port int) server.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	// 命名 profile（heap/goroutine/allocs/block/mutex/threadcreate）
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	return zeushttp.NewHTTP(
		zeushttp.Port(port),
		zeushttp.Mux(mux),
		zeushttp.WithoutAutoClustering(),
	)
}

// NewApp 创建应用（L3 类型装配入口）。
//
// 参数接受三类混合：
//   - AppOption（L3 风格）：WithLogger / WithRegistry / AddServer / ...
//   - components.Component（L4 透传）：components.NewCacheComponent / NewJobComponent / ...
//   - components.AppOption（L4 配置）：components.WithStopTimeout / ...
//
// 内部行为：
//  1. 应用默认值（Name="zeus-service" / Cluster="default" / Logger=slog / Registry=memory）
//  2. 按类型分流：AppOption 应用到 appConfig，其他透传给 components.NewApp
//  3. 转换 appConfig 为 components.NewXxxComponent 列表
//  4. 调用 components.NewApp(comps...) 返回 *components.App
//
// 返回 *components.App：用户能直接调用 .Run() / .RunWithContext(ctx) / .Container() / .Get(name)。
// L3 → L4 升级零成本：在 NewApp(...) 参数末尾追加 components.NewXxxComponent(...) 即可。
//
// 用法：
//
//	a := app.NewApp(
//	    app.AddServer(http.NewHTTP(http.Port(8080), http.Mux(handler))),
//	    app.WithMiddleware(recovery.New()),
//	    app.WithServiceCluster("canary"),
//	)
//	a.Run()
//
// 混用示例（L3 + L4）：
//
//	a := app.NewApp(
//	    app.AddServer(http.NewHTTP()),
//	    components.NewCacheComponent(cache),  // 直接追加 L4 组件
//	)
func NewApp(opts ...any) *components.App {
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		logger:      logslog.NewSlog(),
		registry:    memory.New(),
		stopTimeout: defaultStopTimeoutL3,
	}
	for _, opt := range opts {
		switch v := opt.(type) {
		case nil:
			continue
		case AppOption:
			v(cfg)
		case components.Component, components.AppOption:
			cfg.extraComps = append(cfg.extraComps, v)
		default:
			panic("app.NewApp accepts AppOption / components.Component / components.AppOption, got " + fmt.Sprintf("%T", v))
		}
	}
	comps := buildComponents(cfg)
	return components.NewApp(comps...)
}

// buildComponents 把 L3 配置转换为 L4 components.NewXxxComponent 列表。
//
// 转换规则：
//  1. NewLogComponent / NewRegistryComponent / NewServerComponent / NewServiceComponent 始终装配（核心）
//  2. 每个 middleware 独立 MiddlewareComponent（Name 无冲突）
//  3. Meter/Tracer 仅在非 nil 时装配（避免覆盖用户在 extraComps 中显式注入的）
//  4. cache/database/mq URL → NewFromURL 解析 → 对应 Component（仅当 URL 非空）
//  5. 透传 extraComps（可包含 Component 或 components.AppOption）
//  6. 末尾追加 components.WithStopTimeout（L4 AppOption）
func buildComponents(cfg *appConfig) []any {
	var comps []any

	// 1. 日志
	comps = append(comps, components.NewLogComponent(cfg.logger))

	// 2. 注册中心
	comps = append(comps, components.NewRegistryComponent(cfg.registry))

	// 3. Server（variadic 透传多个）
	if len(cfg.servers) > 0 {
		comps = append(comps, components.NewServerComponent(cfg.servers...))
	}

	// 4. 服务注册（始终装配，透传 Name/Cluster/IP）
	comps = append(comps, components.NewServiceComponent(serviceComponentOpts(cfg)...))

	// 5. 中间件（每个独立 component）
	for _, mw := range cfg.middlewares {
		comps = append(comps, components.NewMiddlewareComponent(mw))
	}

	// 6. Metrics（仅非 nil）
	if cfg.meter != nil {
		comps = append(comps, components.NewMetricsComponent(cfg.meter))
	}

	// 7. Trace（仅非 nil）
	if cfg.tracer != nil {
		comps = append(comps, components.NewTraceComponent(cfg.tracer))
	}

	// 8. cache/database/mq URL（仅当 URL 非空）
	appendURLComponents(&comps, cfg)

	// 9. 透传 L4 组件（L3/L4 混用）
	comps = append(comps, cfg.extraComps...)

	// 10. 优雅关闭超时（L4 AppOption）
	comps = append(comps, components.WithStopTimeout(cfg.stopTimeout))

	return comps
}

// appendURLComponents 把 appConfig 中的可选 URL 解析为 Component 追加到 comps。
//
// 设计原则：
//   - URL 解析失败直接 panic（开发期问题，让用户立刻看到错误，不要等到 Run 时崩溃）
//   - tracer/meter 由 appConfig 已注入的实例自动透传给 database（保持 L3 类型装配的心智）
//   - cache/mq 不需要 tracer/meter（实现内部默认 noop，与 L4 一致）
func appendURLComponents(comps *[]any, cfg *appConfig) {
	if cfg.cacheURL != "" {
		c, err := cache.NewFromURL(cfg.cacheURL)
		if err != nil {
			panic(fmt.Sprintf("app.NewApp: WithCacheURL(%q): %v", cfg.cacheURL, err))
		}
		*comps = append(*comps, components.NewCacheComponent(c))
	}

	if cfg.databaseURL != "" {
		db, err := database.NewFromURL(cfg.databaseURL, cfg.tracer, cfg.meter)
		if err != nil {
			panic(fmt.Sprintf("app.NewApp: WithDatabaseURL(%q): %v", cfg.databaseURL, err))
		}
		*comps = append(*comps, components.NewDatabaseComponent(db))
	}

	if cfg.mqURL != "" {
		broker, err := mq.NewBrokerFromURL(cfg.mqURL)
		if err != nil {
			panic(fmt.Sprintf("app.NewApp: WithMQURL(%q): %v", cfg.mqURL, err))
		}
		*comps = append(*comps, components.NewMQComponent(broker))
	}
}

// serviceComponentOpts 把 appConfig 映射到 ServiceComponent Option
func serviceComponentOpts(cfg *appConfig) []components.ServiceOption {
	opts := []components.ServiceOption{
		components.WithServiceName(cfg.name),
		components.WithServiceCluster(cfg.cluster),
	}
	if cfg.ip != "" {
		opts = append(opts, components.WithServiceIP(cfg.ip))
	}
	return opts
}
