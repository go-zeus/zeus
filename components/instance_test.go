package components

import (
	"github.com/go-zeus/zeus/log"
	"testing"
	"time"
)

func TestGetWaitInstance(t *testing.T) {
	id := "fake"
	fake := NewFake()
	err := SetInstance(id, fake)
	if err != nil {
		return
	}
	go func() {
		fake, err := GetWaitInstance(id)
		log.Info("%v err:%v", fake.IsReady(), err)
	}()
	time.Sleep(3 * time.Second)
	fake.SetReady()
	time.Sleep(time.Second)
}
