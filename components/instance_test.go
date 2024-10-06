package components

import (
	"github.com/go-zeus/zeus/log"
	"testing"
)

func TestGetWaitInstance(t *testing.T) {
	id := "fake"
	err := SetInstance(id, NewFake())
	if err != nil {
		return
	}
	fake := GetWaitInstance(id)
	log.Info("%v", fake)
}
