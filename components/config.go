package components

import (
	"github.com/go-zeus/zeus/config"
)

// ConfigComponent 配置组件适配器
type ConfigComponent struct {
	loader config.Loader
	cfg    *config.Config
}

// NewConfigComponent 创建配置组件
func NewConfigComponent(loader config.Loader) *ConfigComponent {
	return &ConfigComponent{loader: loader}
}

func (c *ConfigComponent) Name() string      { return "config" }
func (c *ConfigComponent) Depends() []string { return nil }

func (c *ConfigComponent) Provide(ctx Context) (any, error) {
	cfg, err := config.NewConfig(c.loader)
	if err != nil {
		return nil, err
	}
	c.cfg = cfg
	return cfg, nil
}

func (c *ConfigComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			if c.cfg != nil {
				return c.cfg.Watch()
			}
			return nil
		},
		OnStop: func(ctx Context) error {
			if c.cfg != nil {
				return c.cfg.Close()
			}
			return nil
		},
	}
}
