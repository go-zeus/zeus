package event

// OneEvent 在消费方未完成的时间里只保留一个事件
type OneEvent interface {
	Trigger()               //发送事件，如果chan里有一个事件则触发的事件会丢弃
	Watch() <-chan struct{} //监听
	Close()                 //关闭
}

func NewOneEvent() OneEvent {
	return &oneEvent{
		ch: make(chan struct{}, 1),
	}
}

type oneEvent struct {
	ch chan struct{}
}

func (d *oneEvent) Trigger() {
	select {
	case d.ch <- struct{}{}:
	default:

	}
}

func (d *oneEvent) Watch() <-chan struct{} {
	return d.ch
}

func (d *oneEvent) Close() {
	close(d.ch)
}
