// Example: mq
//
// 演示声明式消息发布/订阅（MQ）：
//   - 内存 broker + 3 个订阅者（订单事件 / 日志事件 / 通用）
//   - 通过 components 自动装配：注册 + 启动 + 优雅关闭 全部自动化
//   - baggage 全链路传播：发布侧注入 tenant.id，订阅侧自动读取
//
// 启动：
//
//	go run .
//
// 预期输出：
//
//	[INFO] mq demo starting；Ctrl+C to stop
//	[INFO] mq broker started with 3 subscription(s)
//	[INFO] publisher publishing order #1
//	[INFO] [orders.created] order id=1 tenant=acme
//	[INFO] [log.all] captured: order created id=1 tenant=acme
//	[INFO] [audit.all] audit trail: tenant=acme
//	[INFO] publisher publishing order #2
//	...（每秒一组）
//
// Ctrl+C 退出（优雅关闭，broker.Close 等待 in-flight handler 完成）。
package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/log"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/mq/memory"
	"github.com/go-zeus/zeus/propagation"
)

func main() {
	broker := memory.New()

	// 1) 订单创建事件订阅者：处理具体业务
	orderHandler := func(ctx context.Context, msg *mq.Message) error {
		tenant, _ := propagation.Get(ctx, "tenant.id")
		log.Info("[orders.created] order payload=%s tenant=%s", string(msg.Payload), tenant)
		return nil
	}

	// 2) 全量日志订阅者：fan-out 给所有 handler，同一个 topic 也能 fan-out
	logHandler := func(ctx context.Context, msg *mq.Message) error {
		tenant, _ := propagation.Get(ctx, "tenant.id")
		log.Info("[log.all] captured: topic=%s payload=%s tenant=%s",
			msg.Topic, string(msg.Payload), tenant)
		return nil
	}

	// 3) 审计订阅者：演示同 topic 多订阅者 fan-out
	auditHandler := func(ctx context.Context, msg *mq.Message) error {
		tenant, _ := propagation.Get(ctx, "tenant.id")
		log.Info("[audit.all] audit trail: topic=%s tenant=%s", msg.Topic, tenant)
		return nil
	}

	// 装配：MQComponent 持有 broker，每个 NewMQSubscription 声明一个订阅
	// App 启动时自动注册 → Start；停止时自动 Close
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewMQComponent(broker),
		components.NewMQSubscription("orders.created", orderHandler),
		components.NewMQSubscription("log.all", logHandler),
		components.NewMQSubscription("audit.all", auditHandler),
	)

	log.Info("mq demo starting；Ctrl+C to stop")

	// 发布者：在独立 goroutine 周期性发布消息
	// 通过 baggage 注入 tenant.id，订阅侧自动接收
	go func() {
		var seq int64
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for i := 0; ; i++ {
			if i > 0 { // 第 0 次等 ticker 触发，避免启动竞态
				<-ticker.C
			}
			n := atomic.AddInt64(&seq, 1)
			// 发布到 orders.created + log.all + audit.all
			// 用同一 ctx（tenant.id=acme）发布到不同 topic
			ctx := propagation.With(context.Background(), "tenant.id", "acme")

			log.Info("publisher publishing order #%d", n)
			_ = broker.Publish(ctx, "orders.created",
				&mq.Message{Payload: []byte(fmt.Sprintf("order-%d", n))})
			_ = broker.Publish(ctx, "log.all",
				&mq.Message{Payload: []byte(fmt.Sprintf("log-entry-%d", n))})
			_ = broker.Publish(ctx, "audit.all",
				&mq.Message{Payload: []byte(fmt.Sprintf("audit-%d", n))})
		}
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app exited with error: %v\n", err)
		os.Exit(1)
	}
}
