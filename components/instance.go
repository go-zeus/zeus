package components

import (
	"errors"
	"sync"
)

var ins = &instances{data: map[string]Instance{}}

type Instance interface {
	InstanceState
}

type InstanceState interface {
	IsReady() bool
	Wait()
}

type BaseInstance struct {
	cond  *sync.Cond
	ready bool
}

func (b *BaseInstance) IsReady() bool {
	return b.ready
}

func (b *BaseInstance) Wait() {
	for !b.ready {
		b.cond.Wait()
	}
}

type instances struct {
	data map[string]Instance
	mu   sync.RWMutex
}

func SetInstance(id string, instance Instance) error {
	if id == "" {
		return errors.New("实例id不能为空")
	}
	if instance == nil {
		return errors.New("不能设置空实例")
	}
	ins.mu.Lock()
	defer ins.mu.Unlock()
	ins.data[id] = instance
	return nil
}

func GetInstance[T Instance](id string) T {
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	return ins.data[id].(T)
}

func GetWaitInstance(id string) Instance {
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	in := ins.data[id]
	in.Wait()
	return in
}

func DelInstance(kind string) {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	delete(ins.data, kind)
}

func IsReady() bool {
	for _, i := range ins.data {
		if !i.IsReady() {
			return false
		}
	}
	return true
}
