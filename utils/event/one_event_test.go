package event

import (
	"testing"
	"time"
)

func TestOneEvent(t *testing.T) {
	oe := NewOneEvent()
	go func() {
		for ch := range oe.Watch() { // S1000: 用 for range 替代 for { select }
			t.Logf("收到一个信号: %v", ch)
			time.Sleep(5 * time.Second)
		}
	}()
	ti := time.NewTimer(time.Second)
	i := 0
	for range ti.C { // S1000: 用 for range 替代 for { select }
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
