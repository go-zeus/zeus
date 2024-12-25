package components

import "sync"

type Fake struct {
	BaseInstance
}

func NewFake() Instance {
	b := &Fake{}
	b.cond = sync.NewCond(&b.mu)
	return b
}
