package event

// Event 通用事件。
type Event interface {
	Trigger() bool          //触发事件，多次调用安全
	Watch() <-chan struct{} //返回一个通道，Trigger调用后关闭
}

func NewEvent() Event {
	return &event{c: make(chan struct{})}
}

type event struct {
	c chan struct{}
}

func (e *event) Trigger() bool {
	e.c <- struct{}{}
	return true
}

func (e *event) Watch() <-chan struct{} {
	return e.c
}
