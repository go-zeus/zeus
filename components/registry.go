package components

import (
	"io"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/registry"
)

// RegistryComponent 注册中心组件适配器
type RegistryComponent struct {
	reg registry.Registrar
}

// NewRegistryComponent 创建注册中心组件
//
// reg 若额外实现 io.Closer（如 plugins/registry/etcd.New() 的返回值），
// OnStop 时会调用 Close 释放底层连接池，避免依赖 GC 回收造成句柄堆积。
func NewRegistryComponent(d registry.Registrar) *RegistryComponent {
	return &RegistryComponent{reg: d}
}

func (r *RegistryComponent) Name() string      { return "registry" }
func (r *RegistryComponent) Depends() []string { return nil }

func (r *RegistryComponent) Provide(ctx Context) (any, error) {
	return r.reg, nil
}

func (r *RegistryComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStop: func(_ Context) error {
			// 可选 io.Closer 适配：memory registry 无 Close 方法（无资源需释放），
			// etcd 等真实实现持有 client 连接池，需显式关闭
			if closer, ok := r.reg.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					log.Error("registry close failed: %v", err)
				}
			}
			return nil
		},
	}
}
