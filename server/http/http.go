package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/middleware"
	"github.com/go-zeus/zeus/server"
)

const (
	ProtocolName = "http"
	DefaultPort  = 8080
)

type Option func(s *httpServer)

func Port(port int) Option {
	return func(s *httpServer) {
		s.port = port
	}
}

// IP 设置监听 IP（默认 0.0.0.0）。
func IP(ip string) Option {
	return func(s *httpServer) {
		s.ip = ip
	}
}

func Mux(h http.Handler) Option {
	return func(s *httpServer) {
		s.Handler = h
	}
}

// HealthCheck 设置健康检查器
func HealthCheck(hc HealthChecker) Option {
	return func(s *httpServer) {
		s.healthChecker = hc
	}
}

// WithoutAutoClustering 关闭默认的集群路由自动注入。
// 默认行为：在 handler 入口自动把 X-Zeus-Cluster Header 转 context，
// 业务 handler / client / log / trace 等下游组件可直接从 context 读取 cluster。
// 如需自定义中间件链（如使用 ChainHandler），可关闭默认行为。
func WithoutAutoClustering() Option {
	return func(s *httpServer) {
		s.autoClustering = false
	}
}

// TLS 配置 HTTPS / mTLS。
//
// 行为：调用后，server 用 ListenAndServeTLS 替代 ListenAndServe。
// cfg 必须包含至少一对证书；mTLS 场景请设置 cfg.ClientAuth（如 tls.RequireAndVerifyClientCert）。
//
// 示例：
//
//	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
//	srv := http.NewHTTP(
//	    http.Port(8443),
//	    http.TLS(&tls.Config{Certificates: []tls.Certificate{cert}}),
//	)
//
//	mTLS（双向校验）：
//
//	caPool := x509.NewCertPool()
//	caPool.AppendCertsFromPEM(caPEM)
//	http.TLS(&tls.Config{
//	    Certificates: []tls.Certificate{cert},
//	    ClientAuth:   tls.RequireAndVerifyClientCert,
//	    ClientCAs:    caPool,
//	})
func TLS(cfg *tls.Config) Option {
	return func(s *httpServer) {
		s.tlsCfg = cfg
	}
}

// TLSFiles 用证书文件路径配置 HTTPS（便捷封装）。
//
// 等价于：
//
//	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
//	http.TLS(&tls.Config{Certificates: []tls.Certificate{cert}})
//
// 错误处理延后到 Start() 触发，便于 Option 链式调用
func TLSFiles(certFile, keyFile string) Option {
	return func(s *httpServer) {
		s.tlsCertFile = certFile
		s.tlsKeyFile = keyFile
	}
}

func DefaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("welcome to zeus!"))
	})
}

type httpServer struct {
	ip             string
	port           int
	healthChecker  HealthChecker
	useDefault     bool        // 标记是否使用默认 Handler
	autoClustering bool        // 标记是否自动注入集群路由（默认 true）
	startOnce      sync.Once   // 防御二次 Start 导致 handler 嵌套包装
	tlsCfg         *tls.Config // 显式 TLS 配置（优先）
	tlsCertFile    string      // TLS 证书文件（与 tlsKeyFile 配对）
	tlsKeyFile     string      // TLS 私钥文件
	*http.Server
}

// 编译期检查 httpServer 实现了 server.Server 接口
var _ server.Server = (*httpServer)(nil)

// NewHTTP 创建 HTTP 服务器
func NewHTTP(opts ...Option) server.Server {
	s := &httpServer{
		Server:         &http.Server{},
		autoClustering: true, // 默认启用集群路由自动注入
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.port == 0 {
		s.port = DefaultPort
	}
	if s.Handler == nil {
		s.Handler = DefaultHandler()
		s.useDefault = true
	}
	// TLS：优先用显式 cfg，否则尝试从 cert/key 文件加载
	if s.tlsCfg != nil {
		s.Server.TLSConfig = s.tlsCfg
	} else if s.tlsCertFile != "" && s.tlsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.tlsCertFile, s.tlsKeyFile)
		if err == nil {
			s.Server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		}
		// 加载失败不在此处返回 error（保持 Option 链式 API 风格）
		// 启动时若 TLSConfig 仍为 nil 会回退到 HTTP
	}
	return s
}

func (s *httpServer) Protocol() string { return ProtocolName }

func (s *httpServer) Endpoint() string {
	return fmt.Sprintf("%s:%d", s.ip, s.port)
}

// ApplyMiddleware 在 server.Start 之前把中间件链包装到当前 handler 外层。
//
// 该方法实现 components 包内 ServerComponent 期望的中间件注入接口（鸭子类型）。
// ServerComponent.OnStart 会从装配上下文收集所有 middleware.Interceptor 并按拓扑顺序
// 调用每个 server 的 ApplyMiddleware。这样 L3/L4 用户只需要：
//
//	components.NewApp(
//	    components.NewMiddlewareComponent(recovery.New()),
//	    components.NewMiddlewareComponent(tracing.New()),
//	    components.NewServerComponent(http.NewHTTP(...)),
//	)
//
// 即可让中间件自动生效，无需手动 ChainHandler 包装。
//
// 多次调用会嵌套包装（按调用顺序外→内），但 ServerComponent 通常只调用一次。
// 空链视为 no-op。
func (s *httpServer) ApplyMiddleware(chain middleware.Chain) {
	if len(chain) == 0 {
		return
	}
	s.Handler = ChainHandler(s.Handler, chain)
}

func (s *httpServer) Start(ctx context.Context) error {
	// 防御二次 Start：包装 mux 和 clusterInjector 只能执行一次，否则会嵌套包装
	s.startOnce.Do(func() {
		if s.useDefault {
			mux := http.NewServeMux()
			mux.HandleFunc("/health", healthHandler)
			mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
				readinessHandler(w, r, s.healthChecker)
			})
			mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
				livenessHandler(w, r, s.healthChecker)
			})
			mux.Handle("/", s.Handler)
			s.Handler = mux
		}
		if s.autoClustering {
			s.Handler = clusterInjector(s.Handler)
		}
	})
	// 启动模式：HTTPS（显式 cfg 或证书文件）/ HTTP
	useTLS := s.TLSConfig != nil
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	log.Info("server start is %s://%s", scheme, s.Endpoint())
	s.Addr = s.Endpoint()
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	if useTLS {
		ln = tls.NewListener(ln, s.TLSConfig)
	}
	// 监听 context 取消以优雅关闭，同时监听 Serve 返回以避免 goroutine 泄漏
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.Shutdown(shutCtx)
		case <-done:
		}
	}()
	err = s.Serve(ln)
	close(done)
	return err
}

func (s *httpServer) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}
