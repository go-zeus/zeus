package components

import (
	"github.com/go-zeus/zeus/log"
)

// LogComponent 日志组件适配器
type LogComponent struct {
	writer log.Writer
	logger *log.Logger
}

// NewLogComponent 创建日志组件
func NewLogComponent(w log.Writer) *LogComponent {
	return &LogComponent{writer: w}
}

func (l *LogComponent) Name() string      { return "log" }
func (l *LogComponent) Depends() []string { return nil }

func (l *LogComponent) Provide(ctx Context) (any, error) {
	l.logger = log.NewLogger(l.writer)
	log.SetDefault(l.logger)
	return l.logger, nil
}

func (l *LogComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error { return nil },
		OnStop: func(ctx Context) error {
			if l.logger != nil {
				return l.logger.Close()
			}
			return nil
		},
	}
}
