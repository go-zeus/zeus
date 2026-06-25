package components

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/mq"
)

// MQComponent 消息代理组件适配器。
//
// 职责：
//   - 持有 mq.Broker 实例
//   - OnStart 时把所有 MQSubscription 收集的 (topic, handler) 注册到 Broker
//   - OnStop 时优雅关闭（Broker.Close）
//
// 与 MQSubscription 配合使用，实现声明式消息订阅：
//
//	components.NewApp(
//	    components.NewMQComponent(memory.New()),
//	    components.NewMQSubscription("orders.created", handleOrder),
//	    components.NewMQSubscription("user.signup", handleSignup),
//	)
//
// 发布消息：业务代码通过 Type[mq.Broker](ctx) 或 ctx.Get("mq") 获取 broker 实例，
// 调用 broker.Publish(ctx, topic, msg)。
//
// App 启动时自动注册所有订阅；停止时优雅关闭 broker。
type MQComponent struct {
	broker mq.Broker
}

// NewMQComponent 创建消息代理组件。
// broker 为 nil 时返回的组件为 no-op（订阅仍会被收集但不会真正订阅）。
func NewMQComponent(broker mq.Broker) *MQComponent {
	return &MQComponent{broker: broker}
}

func (m *MQComponent) Name() string      { return "mq" }
func (m *MQComponent) Depends() []string { return nil }

// Provide 把 Broker 实例发布到容器，供其他组件通过 ctx.Get("mq") 取用
func (m *MQComponent) Provide(_ Context) (any, error) {
	return m.broker, nil
}

// Lifecycle OnStart 收集所有 MQSubscription 并注册到 Broker；
// OnStop 优雅关闭（取消所有订阅 ctx，等待 in-flight handler 完成）。
func (m *MQComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			if m.broker == nil {
				return nil // 无 Broker 时 no-op
			}
			// 收集所有 MQSubscription（按注册顺序）
			subs, err := AllByType[*MQSubscription](ctx)
			if err != nil {
				return err
			}
			for _, s := range subs {
				if err := m.broker.Subscribe(ctx, s.topic, s.handler); err != nil {
					return err
				}
			}
			log.Info("mq broker started with %d subscription(s)", len(subs))
			return nil
		},
		OnStop: func(_ Context) error {
			if m.broker == nil {
				return nil
			}
			return m.broker.Close()
		},
	}
}

// MQSubscription 单个订阅的注册装饰器。
//
// 不直接调 Broker.Subscribe，而是通过 Provide 把自己发布到容器，
// 让 MQComponent.OnStart 统一收集并注册（保证 Broker 已就绪）。
//
// Name 在 topic 后追加全局自增序号：同 topic 多次注册不会冲突。
type MQSubscription struct {
	topic   string
	handler mq.Handler
	id      int64
}

// subSeq 全局订阅序号，用于让 Name() 唯一
var subSeq int64

// NewMQSubscription 包装单个 (topic, handler) 为组件。
// 多个 MQSubscription 可同时注册到 NewApp（同 topic 也支持 fan-out）。
// handler 不能为 nil（注册时 Broker 会校验）。
func NewMQSubscription(topic string, handler mq.Handler) *MQSubscription {
	return &MQSubscription{
		topic:   topic,
		handler: handler,
		id:      atomic.AddInt64(&subSeq, 1),
	}
}

func (s *MQSubscription) Name() string {
	// topic + 自增序号，保证同 topic 多次注册时 Name 不冲突
	return "mq_subscription:" + s.topic + ":" + strconv.FormatInt(s.id, 10)
}
func (s *MQSubscription) Depends() []string { return []string{"mq"} }

// Provide 返回自身指针，MQComponent.OnStart 通过 AllByType[*MQSubscription] 收集
func (s *MQSubscription) Provide(_ Context) (any, error) {
	return s, nil
}

// Lifecycle 不做事，全部由 MQComponent 统一编排
func (s *MQSubscription) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(_ Context) error {
			_ = context.Background()
			return nil
		},
	}
}
