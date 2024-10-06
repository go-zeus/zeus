package event

import (
	"testing"
	"time"
)

func TestOneEvent(t *testing.T) {
	oe := NewOneEvent()
	go func() {
		for {
			select {
			case <-oe.Watch():
				t.Logf("收到一个信号")
				time.Sleep(5 * time.Second)
			}
		}
	}()
	ti := time.NewTimer(time.Second)
	i := 0
	for {
		select {
		case <-ti.C:
			oe.Trigger()
			if i > 20 {
				oe.Close()
				t.Logf("oe.Close()")
				return
			}
			i++
			ti.Reset(time.Second)
		}
	}
}
