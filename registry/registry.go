package registry

import (
	"github.com/go-zeus/zeus/types"
)

// Registry 注册中心
type Registry interface {
	Register
	Discovery
	String() string
}

// Register 服务注册
type Register interface {
	Register(ins *types.Instance) (err error)
	Deregister(ins *types.Instance) (err error)
}

type Discovery interface {
	Watcher
	Storage
}

// Watcher 服务监听
type Watcher interface {
	Watch(serviceName string) <-chan struct{}
}
