package server

import "context"

// Server 服务器接口
//
// 具体实现：
//   - server/http 包：HTTP/HTTPS 服务器（构造函数 NewHTTP）
//   - plugins/server/grpc：gRPC 服务器
//
// 用户应通过 components.NewServerComponent(http.NewHTTP(...)) 显式装配，
// 而不是依赖全局默认 server。L1 用户使用 app.Run(cfg, handler) 入口，
// 内部按 handler 类型自动选择 server 实现。
type Server interface {
	Protocol() string
	Endpoint() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
