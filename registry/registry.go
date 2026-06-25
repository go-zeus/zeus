package registry

import (
	"context"

	"github.com/go-zeus/zeus/types"
)

// Registrar 服务注册接口
type Registrar interface {
	Register(ctx context.Context, ins *types.Instance) error
	Deregister(ctx context.Context, ins *types.Instance) error
}

// Watcher 服务监听接口
type Watcher interface {
	Watch(ctx context.Context, serviceName string) (<-chan struct{}, error)
}

// Discovery 服务发现接口
type Discovery interface {
	GetService(ctx context.Context, serviceName string) (*types.ServiceEntry, error)
}
