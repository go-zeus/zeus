package components

import (
	"context"
	"fmt"
	"time"

	"github.com/go-zeus/zeus/database"
	"github.com/go-zeus/zeus/log"
)

// DatabaseComponent 数据库组件适配器。
//
// 职责：
//   - 持有 database.DB 实例
//   - OnStart 时调 Ping 验证连接
//   - OnStop 时 Close 释放连接池
//
// 与底层实现解耦：用户可在 New 时注入任意 database.DB 实现
// （database/sql 包装 或 plugins/database/mysql）。
//
// 用法：
//
//	db, _ := sql.New(database.DBOptions{Driver: "mysql", DSN: ...}, tracer, meter)
//	app := components.NewApp(
//	    components.NewDatabaseComponent(db),
//	    // 业务组件通过 Type[database.DB](ctx) 取 db 实例
//	)
type DatabaseComponent struct {
	db          database.DB
	pingOnStart bool
	pingTimeout time.Duration
}

// DatabaseOption 配置 DatabaseComponent
type DatabaseOption func(*DatabaseComponent)

// WithPingOnStart 启动时 Ping 验证连接（默认开启）
func WithPingOnStart(b bool) DatabaseOption {
	return func(c *DatabaseComponent) { c.pingOnStart = b }
}

// WithPingTimeout Ping 超时时间（默认 5s）
func WithPingTimeout(d time.Duration) DatabaseOption {
	return func(c *DatabaseComponent) {
		if d > 0 {
			c.pingTimeout = d
		}
	}
}

// NewDatabaseComponent 创建数据库组件
//
// db 为 nil 时返回的组件为 no-op
func NewDatabaseComponent(db database.DB, opts ...DatabaseOption) *DatabaseComponent {
	c := &DatabaseComponent{
		db:          db,
		pingOnStart: true,
		pingTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

func (c *DatabaseComponent) Name() string      { return "database" }
func (c *DatabaseComponent) Depends() []string { return nil }

// Provide 把 DB 实例发布到容器，供其他组件通过 Type[database.DB](ctx) 取用
func (c *DatabaseComponent) Provide(_ Context) (any, error) {
	return c.db, nil
}

// Lifecycle OnStart 调 Ping；OnStop 调 Close
func (c *DatabaseComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(_ Context) error {
			if c.db == nil {
				return nil
			}
			if !c.pingOnStart {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), c.pingTimeout)
			defer cancel()
			if err := c.db.Ping(ctx); err != nil {
				return fmt.Errorf("database: ping failed: %w", err)
			}
			log.Info("database connected (ping ok)")
			return nil
		},
		OnStop: func(_ Context) error {
			if c.db == nil {
				return nil
			}
			return c.db.Close()
		},
	}
}
