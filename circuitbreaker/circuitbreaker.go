package circuitbreaker

// State 熔断器状态

// State 熔断器状态
type State int

const (
	StateClosed   State = iota // 正常，允许请求通过
	StateHalfOpen              // 半开，允许少量请求探测
	StateOpen                  // 打开，拒绝所有请求
)

// Breaker 熔断器接口（实现者实现此接口）
type Breaker interface {
	Allow() error
	MarkSuccess()
	MarkFailed()
	State() State
}

// CircuitBreaker 熔断器（用户 API）
type CircuitBreaker struct {
	breaker Breaker
}

// NewCircuitBreaker 从 Breaker 创建熔断器
func NewCircuitBreaker(b Breaker) *CircuitBreaker {
	return &CircuitBreaker{breaker: b}
}

func (cb *CircuitBreaker) Allow() error { return cb.breaker.Allow() }
func (cb *CircuitBreaker) MarkSuccess() { cb.breaker.MarkSuccess() }
func (cb *CircuitBreaker) MarkFailed()  { cb.breaker.MarkFailed() }
func (cb *CircuitBreaker) State() State { return cb.breaker.State() }

// Execute 执行函数，自动标记成功/失败
//
// fn panic 时：Allow 已消耗一个半开探测名额，必须调用 MarkFailed 回退状态，
// 否则半开计数卡死、后续请求被拒直至超时重置。恢复状态后重新抛出 panic，
// 不掩盖业务 bug。
func (cb *CircuitBreaker) Execute(fn func() error) (err error) {
	if e := cb.Allow(); e != nil {
		return e
	}
	defer func() {
		if r := recover(); r != nil {
			cb.MarkFailed()
			panic(r)
		}
	}()
	err = fn()
	if err != nil {
		cb.MarkFailed()
		return err
	}
	cb.MarkSuccess()
	return nil
}
