package components

import "sync"

type Fake struct {
	BaseInstance
}

func NewFake() Instance {
	return &Fake{BaseInstance{
		cond:  sync.NewCond(&sync.Mutex{}),
		ready: false,
	}}
}
